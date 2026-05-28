package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/model"
	"github.com/find-assets/scanner/internal/service"
)

// ScanHandler 持有 Gin 路由所需的依赖。
type ScanHandler struct {
	svc      *service.ScanService
	store    *TaskStore
	running  atomic.Int32 // 当前正在运行的任务数（限制 = 1）
	maxConc  int32
	bgCtx    context.Context
	bgCancel context.CancelFunc
}

func NewScanHandler(svc *service.ScanService, store *TaskStore) *ScanHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &ScanHandler{
		svc: svc, store: store, maxConc: 1,
		bgCtx: ctx, bgCancel: cancel,
	}
}

// Shutdown 触发后台任务退出（程序退出时调用）。
func (h *ScanHandler) Shutdown() { h.bgCancel() }

// ScanRequest HTTP 请求体。
type ScanRequest struct {
	Mode    string `json:"mode" binding:"required,oneof=day week"`
	Workers int    `json:"workers,omitempty"`
	// Range 日线策略均线粘合度阈值，百分比单位（例如 1.5 表示 1.5%）。优先于 Cohesion。
	Range float64 `json:"range,omitempty"`
	// Cohesion 同 Range 的小数形式（例如 0.015）。仅在 Range 未填时生效。
	Cohesion  float64 `json:"cohesion,omitempty"`
	BarsLimit int     `json:"bars_limit,omitempty"`
}

// SyncScan 同步执行一次扫描，直接返回 JSON 报告。
func (h *ScanHandler) SyncScan(c *gin.Context) {
	var req ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.running.CompareAndSwap(0, h.maxConc) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "已有扫描任务在执行，请稍后再试"})
		return
	}
	defer h.running.Store(0)

	rep, err := h.svc.Run(c.Request.Context(), service.Params{
		Mode:      req.Mode,
		Workers:   req.Workers,
		BarsLimit: req.BarsLimit,
		Range:     req.Range,
		Cohesion:  req.Cohesion,
		TaskID:    uuid.NewString(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rep)
}

// AsyncScan 立即返回 task_id，后台运行。
func (h *ScanHandler) AsyncScan(c *gin.Context) {
	var req ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.running.CompareAndSwap(0, h.maxConc) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "已有扫描任务在执行，请稍后再试"})
		return
	}

	task := newTask(uuid.NewString(), req.Mode)
	task.Status = StatusRunning
	h.store.Put(task)

	go func() {
		defer h.running.Store(0)
		defer task.markFinished()

		rep, err := h.svc.Run(h.bgCtx, service.Params{
			Mode:      req.Mode,
			Workers:   req.Workers,
			BarsLimit: req.BarsLimit,
			Range:     req.Range,
			Cohesion:  req.Cohesion,
			TaskID:    task.ID,
			OnStocks: func(total int, _ []model.Stock) {
				atomic.StoreInt64(&task.Total, int64(total))
				task.publish(ProgressEvent{
					Total: int64(total), Stage: "scanning",
					Message: fmt.Sprintf("获取到 %d 只股票", total),
				})
			},
			Progress: func(done, total int64) {
				atomic.StoreInt64(&task.Done, done)
				atomic.StoreInt64(&task.Total, total)
				if done == total || done%200 == 0 {
					task.publish(ProgressEvent{
						Done: done, Total: total, Stage: "scanning",
					})
				}
			},
		})
		task.FinishedAt = time.Now()
		if err != nil {
			task.Status = StatusFailed
			task.Err = err.Error()
			task.publish(ProgressEvent{Stage: "finished", Message: err.Error()})
			return
		}
		task.Report = rep
		task.Status = StatusDone
		task.publish(ProgressEvent{
			Done: task.Done, Total: task.Total,
			Stage: "finished", Message: "ok",
		})
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"task_id": task.ID,
		"status":  task.Status,
	})
}

// GetResult 查询异步任务状态 / 结果。
func (h *ScanHandler) GetResult(c *gin.Context) {
	task := h.store.Get(c.Param("id"))
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	resp := gin.H{
		"task_id":     task.ID,
		"mode":        task.Mode,
		"status":      task.Status,
		"done":        atomic.LoadInt64(&task.Done),
		"total":       atomic.LoadInt64(&task.Total),
		"started_at":  task.StartedAt,
		"finished_at": task.FinishedAt,
		"error":       task.Err,
	}
	if task.Report != nil {
		resp["report"] = task.Report
	}
	c.JSON(http.StatusOK, resp)
}

// Stream 通过 SSE 推送任务进度。
func (h *ScanHandler) Stream(c *gin.Context) {
	task := h.store.Get(c.Param("id"))
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ch := task.Subscribe()
	flusher, _ := c.Writer.(http.Flusher)
	send := func(ev string, payload any) bool {
		b, _ := json.Marshal(payload)
		if ev != "" {
			fmt.Fprintf(c.Writer, "event: %s\n", ev)
		}
		_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
		return err == nil
	}

	// 首帧推送当前快照
	if !send("snapshot", gin.H{
		"task_id": task.ID,
		"status":  task.Status,
		"done":    atomic.LoadInt64(&task.Done),
		"total":   atomic.LoadInt64(&task.Total),
	}) {
		return
	}

	for {
		select {
		case ev := <-ch:
			if !send("progress", ev) {
				return
			}
		case <-task.Finished():
			send("end", gin.H{
				"status": task.Status, "error": task.Err,
			})
			return
		case <-c.Request.Context().Done():
			return
		}
	}
}

// Export 按 format 下载报告。
func (h *ScanHandler) Export(c *gin.Context) {
	task := h.store.Get(c.Param("id"))
	if task == nil || task.Report == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "结果不存在或尚未完成"})
		return
	}
	format := c.DefaultQuery("format", "json")
	exp := exporter.Get(format)
	if exp == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未知导出格式: " + format})
		return
	}
	c.Header("Content-Type", exp.ContentType())
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="scan_%s_%s.%s"`,
		task.Mode, task.StartedAt.Format("20060102_150405"), format,
	))
	_ = exp.Write(c.Writer, task.Report)
}

// ListStrategies 列出当前已注册的策略。
func (h *ScanHandler) ListStrategies(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"strategies": []gin.H{
			{"mode": "day", "title": "日线一箭穿心"},
			{"mode": "week", "title": "周线超跌拐点"},
		},
	})
}
