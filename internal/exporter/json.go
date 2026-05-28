package exporter

import (
	"encoding/json"
	"io"
)

type JSON struct {
	Pretty bool
}

func (JSON) Format() string      { return "json" }
func (JSON) ContentType() string { return "application/json; charset=utf-8" }

func (j JSON) Write(w io.Writer, r *Report) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if j.Pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(r)
}
