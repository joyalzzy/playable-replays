package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
)

func main() {
	path := flag.String("path", "../fixtures/moments.json", "path to the authored fixture pack")
	flag.Parse()

	moments, err := fixtures.Load(*path)
	if err != nil {
		fail(err)
	}
	results := fixtures.RunAcceptanceTests(moments)
	failed := 0
	for _, result := range results {
		if result.Passed {
			continue
		}
		failed++
		fmt.Fprintf(os.Stderr, "FAIL %s / %s: %s\n", result.MomentID, result.TestName, result.Detail)
	}
	if failed > 0 {
		fail(fmt.Errorf("%d of %d acceptance tests failed", failed, len(results)))
	}

	categories := map[string]int{}
	levels := map[string]int{}
	for _, moment := range moments {
		categories[moment.Authoring.Category]++
		levels[moment.Authoring.SkillLevel]++
	}
	fmt.Printf("Validated %d authored scenarios and %d deterministic acceptance tests.\n", len(moments), len(results))
	printCoverage("Category coverage", categories)
	printCoverage("Skill coverage", levels)
}

func printCoverage(label string, counts map[string]int) {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Println(label + ":")
	for _, key := range keys {
		fmt.Printf("  %s: %d\n", key, counts[key])
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fixture validation failed:", err)
	os.Exit(1)
}
