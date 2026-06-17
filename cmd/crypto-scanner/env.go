package main

import (
	"bufio"
	"os"
	"strings"
)

const defaultEnvFile = ".env"

func resolveEnvFileArg(args []string) (path string, explicit bool) {
	for i, arg := range args {
		if arg == "-env" && i+1 < len(args) {
			return args[i+1], true
		}
		if strings.HasPrefix(arg, "-env=") {
			return strings.TrimPrefix(arg, "-env="), true
		}
	}
	return defaultEnvFile, false
}

func loadEnvFile(path string, explicit bool) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	fp, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return nil
		}
		return err
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		if existing, ok := os.LookupEnv(key); ok && existing != "" {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return err
		}
	}
	return scanner.Err()
}
