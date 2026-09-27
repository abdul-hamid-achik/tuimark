package main

import "os"

// demoFiles are written into a freshly (re)created demo workspace so specs
// and manual runs start from a known state.
var demoFiles = map[string]string{
	"notes.txt": "before\n",
	"main.go":   "package main\n\nfunc main() {}\n",
}

// SeedDemoWorkspace recreates dir empty and writes demoFiles into it.
func SeedDemoWorkspace(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, content := range demoFiles {
		if err := os.WriteFile(dir+string(os.PathSeparator)+name, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
