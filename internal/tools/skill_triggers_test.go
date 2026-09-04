package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Skill routing itself is an LLM decision, so no Go test can assert that a
// question actually selects the right skill. What a Go test CAN hold is the
// scaffolding that routing depends on: every skill states when to use it, and
// the trigger test set stays in sync with the skills that exist. Without this,
// a new skill ships with no trigger coverage and nobody notices until a user's
// agent picks the wrong one.
//
// The judged run lives outside CI: hand testdata/skill_triggers.json and the
// six name+description pairs (never the bodies) to an agent that has not read
// this repo, and compare its picks against expect.

// minCasesPerSkill is a floor, not a target. Six phrasings is enough to catch a
// description that only answers to the one wording its author had in mind.
const minCasesPerSkill = 4

var triggerClause = regexp.MustCompile(`Use when |Consult this whenever `)

type triggerCase struct {
	Q      string `json:"q"`
	Expect string `json:"expect"`
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestSkillTriggers_EverySkillSaysWhenToUseIt(t *testing.T) {
	t.Parallel()

	for name, desc := range skillDescriptions(t) {
		if !triggerClause.MatchString(desc) {
			t.Errorf("%s: description has no trigger clause. A description that only "+
				"says what the skill does leaves the client guessing when to reach for it; "+
				"spell out the questions it answers with \"Use when asked ...\"", name)
		}
	}
}

func TestSkillTriggers_TestSetCoversEverySkill(t *testing.T) {
	t.Parallel()

	skills := skillDescriptions(t)
	cases := triggerCases(t)

	counts := map[string]int{}
	negatives := 0
	for _, c := range cases {
		if c.Expect == "none" {
			negatives++
			continue
		}
		if _, ok := skills[c.Expect]; !ok {
			t.Errorf("case %q expects skill %q, which has no directory under .agents/skills/", c.Q, c.Expect)
			continue
		}
		counts[c.Expect]++
	}

	for name := range skills {
		if counts[name] < minCasesPerSkill {
			t.Errorf("skill %s has %d trigger cases, want at least %d. Add phrasings to "+
				"internal/tools/testdata/skill_triggers.json — a skill nobody tested the "+
				"wording of is a skill that fires on the author's phrasing only",
				name, counts[name], minCasesPerSkill)
		}
	}

	// Without cases that must select nothing, a description that grabs every
	// question scores perfectly.
	if negatives == 0 {
		t.Error("trigger test set has no expect:\"none\" cases; add questions that must select no skill at all")
	}
}

// skillDescriptions maps skill directory name to the description line in its
// SKILL.md front matter.
func skillDescriptions(t *testing.T) map[string]string {
	t.Helper()

	dir := filepath.Join(repoRoot(t), ".agents", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	out := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		desc := ""
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "description:") {
				desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				break
			}
		}
		if desc == "" {
			t.Fatalf("%s: front matter has no description line", path)
		}
		out[e.Name()] = desc
	}
	if len(out) == 0 {
		t.Fatalf("no skills found under %s", dir)
	}
	return out
}

func triggerCases(t *testing.T) []triggerCase {
	t.Helper()

	path := filepath.Join(repoRoot(t), "internal", "tools", "testdata", "skill_triggers.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		Cases []triggerCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(doc.Cases) == 0 {
		t.Fatalf("%s has no cases", path)
	}
	return doc.Cases
}
