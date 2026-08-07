package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joyalzzy/playable-replays/backend/internal/drafts"
	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "scenario draft:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected create, validate, publish, or preview")
	}
	switch args[0] {
	case "create":
		return create(args[1:])
	case "validate":
		return validate(args[1:])
	case "publish":
		return publish(args[1:], false)
	case "preview":
		return publish(args[1:], true)
	default:
		return fmt.Errorf("unknown command %q; expected create, validate, publish, or preview", args[0])
	}
}

func create(args []string) error {
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	input := flags.String("input", "", "telemetry detector NDJSON path")
	output := flags.String("output", "", "version 2.1 draft bundle path")
	force := flags.Bool("force", false, "replace an existing output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return errors.New("create requires --input and --output")
	}
	file, err := os.Open(*input)
	if err != nil {
		return fmt.Errorf("open detector NDJSON: %w", err)
	}
	defer file.Close()
	bundle, err := drafts.FromNDJSON(file)
	if err != nil {
		return err
	}
	if err := drafts.WriteBundle(*output, bundle, *force); err != nil {
		return err
	}
	fmt.Printf("Created %d intentionally incomplete scenario draft(s) in %s.\n", len(bundle.Drafts), *output)
	fmt.Println("Complete the synthetic units, rules, skill level, analyst rationale, tradeoffs, alternatives, and acceptance tests before validation.")
	return nil
}

func validate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	draftPath := flags.String("draft", "", "version 2.1 draft bundle path")
	index := flags.Int("index", 0, "zero-based draft index")
	if err := flags.Parse(args); err != nil {
		return err
	}
	draft, err := loadDraft(*draftPath, *index)
	if err != nil {
		return err
	}
	if err := drafts.ValidateDraft(draft); err != nil {
		return err
	}
	fmt.Printf("Draft %d (%s) is complete and publishable.\n", *index, draft.Scenario.Slug)
	return nil
}

func publish(args []string, preview bool) error {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	draftPath := flags.String("draft", "", "version 2.1 draft bundle path")
	index := flags.Int("index", 0, "zero-based draft index")
	basePath := flags.String("base", "../fixtures/moments.json", "validated base fixture pack")
	output := flags.String("output", "", "new fixture pack path")
	force := flags.Bool("force", false, "replace an existing output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("publish and preview require --output")
	}
	draft, err := loadDraft(*draftPath, *index)
	if err != nil {
		return err
	}
	baseAbsolute, err := filepath.Abs(*basePath)
	if err != nil {
		return fmt.Errorf("resolve base pack: %w", err)
	}
	outputAbsolute, err := filepath.Abs(*output)
	if err != nil {
		return fmt.Errorf("resolve output pack: %w", err)
	}
	if filepath.Clean(baseAbsolute) == filepath.Clean(outputAbsolute) {
		return errors.New("refusing to overwrite the base fixture pack; publish to a reviewable output path")
	}
	base, err := fixtures.Load(*basePath)
	if err != nil {
		return fmt.Errorf("load base pack: %w", err)
	}
	moments, err := drafts.PreparePack(draft, base)
	if err != nil {
		return err
	}
	if err := drafts.WritePack(*output, moments, *force); err != nil {
		return err
	}
	if !preview {
		fmt.Printf("Published validated scenario %s into review pack %s.\n", draft.Scenario.ID, *output)
		return nil
	}
	fmt.Printf("Preview pack ready at %s.\n", outputAbsolute)
	fmt.Println("From backend/ in PowerShell:")
	fmt.Printf("  $env:FIXTURE_PATH='%s'; go run ./cmd/server\n", outputAbsolute)
	fmt.Println("In another terminal, from frontend/:")
	fmt.Println("  npm install; npm run dev")
	fmt.Printf("Open http://127.0.0.1:5173/?moment=%s\n", draft.Scenario.Slug)
	return nil
}

func loadDraft(path string, index int) (drafts.Draft, error) {
	if path == "" {
		return drafts.Draft{}, errors.New("--draft is required")
	}
	if index < 0 {
		return drafts.Draft{}, errors.New("--index cannot be negative")
	}
	file, err := os.Open(path)
	if err != nil {
		return drafts.Draft{}, fmt.Errorf("open draft bundle: %w", err)
	}
	defer file.Close()
	bundle, err := drafts.ReadBundle(file)
	if err != nil {
		return drafts.Draft{}, err
	}
	if index >= len(bundle.Drafts) {
		return drafts.Draft{}, fmt.Errorf("draft index %d is outside 0..%d", index, len(bundle.Drafts)-1)
	}
	return bundle.Drafts[index], nil
}
