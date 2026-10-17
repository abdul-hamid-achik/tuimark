package play

// TEMPORARY: a CPU profile of cases a and c on the CI runner, printed to
// the bench job's log, removed before the branch is done.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

var profOnce sync.Once

func profileCase(name string, iters int, setup func() (step func(i int))) {
	step := setup()
	dir, _ := os.MkdirTemp("", "prof")
	path := filepath.Join(dir, name+".pprof")
	f, err := os.Create(path)
	if err != nil {
		fmt.Println("profile:", err)
		return
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Println("profile:", err)
		return
	}
	for i := 0; i < iters; i++ {
		step(i)
	}
	pprof.StopCPUProfile()
	f.Close()
	for _, args := range [][]string{
		{"tool", "pprof", "-top", "-nodecount=40", os.Args[0], path},
		{"tool", "pprof", "-top", "-cum", "-nodecount=60", os.Args[0], path},
	} {
		out, err := exec.Command("go", args...).CombinedOutput()
		fmt.Printf("=== PROFILE %s %s (err %v)\n", name, strings.Join(args[2:4], " "), err)
		for _, l := range strings.Split(string(out), "\n") {
			fmt.Println("PROF|", l)
		}
	}
}

func BenchmarkZZProfile(b *testing.B) {
	profOnce.Do(func() {
		profileCase("down", 400, func() func(int) {
			s := benchSession(b, benchList)
			down := KeyFromToken("down")
			return func(i int) {
				s.handleKeys(i+1, []host.Key{down})
				s.settle(i + 1)
			}
		})
		profileCase("derive", 400, func() func(int) {
			a, err := host.Parse(strings.NewReader(benchFilter))
			if err != nil {
				b.Fatal(err)
			}
			data := benchData(benchRows)
			data["q"] = ""
			if err := a.Bind("", data); err != nil {
				b.Fatal(err)
			}
			if err := a.Derive("visible", filterRows); err != nil {
				b.Fatal(err)
			}
			s := NewSession(a, 80, 24, false, false, false, true)
			s.Run(nil)
			const query = "123"
			return func(i int) {
				if i > 0 && i%len(query) == 0 {
					_ = s.App.Set("q", "")
					s.settle(i)
				}
				s.handleText(i+1, query[i%len(query):i%len(query)+1])
				s.settle(i + 1)
			}
		})
	})
	for i := 0; i < b.N; i++ {
	}
}
