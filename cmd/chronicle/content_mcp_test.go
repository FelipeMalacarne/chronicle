package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/annotate"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
)

// copyTree copies the files of src into dst.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mcpClient drives a server over pipes, one request at a time.
type mcpClient struct {
	t   *testing.T
	in  io.Writer
	out *bufio.Scanner
	id  int
}

func (c *mcpClient) call(method string, params any) json.RawMessage {
	c.t.Helper()
	c.id++
	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	if _, err := c.in.Write(append(req, '\n')); err != nil {
		c.t.Fatal(err)
	}
	if !c.out.Scan() {
		c.t.Fatalf("%s: no response (%v)", method, c.out.Err())
	}
	var resp struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct{ Message string }
	}
	if err := json.Unmarshal(c.out.Bytes(), &resp); err != nil {
		c.t.Fatal(err)
	}
	if resp.Error != nil || resp.ID != c.id {
		c.t.Fatalf("%s: id %d error %+v", method, resp.ID, resp.Error)
	}
	return resp.Result
}

// tool calls a tool and decodes its JSON text into out.
func (c *mcpClient) tool(name string, args, out any) bool {
	c.t.Helper()
	var res struct {
		Content []struct{ Text string }
		IsError bool
	}
	if err := json.Unmarshal(c.call("tools/call", map[string]any{"name": name, "arguments": args}), &res); err != nil {
		c.t.Fatal(err)
	}
	if res.IsError {
		c.t.Logf("%s: tool error: %s", name, res.Content[0].Text)
		return false
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), out); err != nil {
		c.t.Fatalf("%s: %v in %s", name, err, res.Content[0].Text)
	}
	return true
}

// TestContentMCPExampleBatch runs the extraction loop over stdio against a
// copy of the real Oziris guide: initialize, list tools, take a batch,
// resolve a name, submit, then check the loader composes what was written.
func TestContentMCPExampleBatch(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "content", "reference"), filepath.Join(dir, "reference"))
	copyTree(t, filepath.Join("..", "..", "content", "guides", "oziris-main"), filepath.Join(dir, "guides", "oziris-main"))
	_ = os.RemoveAll(filepath.Join(dir, "guides", "oziris-main", content.AnnotationsSubdir))

	ws, err := annotate.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- newContentMCPServer(ws).Serve(context.Background(), inR, outW); outW.Close() }()
	c := &mcpClient{t: t, in: inW, out: bufio.NewScanner(outR)}
	c.out.Buffer(nil, 16<<20)

	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}}), &init)
	if init.ProtocolVersion != "2025-06-18" {
		t.Errorf("protocol = %q", init.ProtocolVersion)
	}
	if _, werr := inW.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")); werr != nil {
		t.Fatal(werr)
	}
	var list struct{ Tools []struct{ Name string } }
	_ = json.Unmarshal(c.call("tools/list", map[string]any{}), &list)
	var names []string
	for _, tl := range list.Tools {
		names = append(names, tl.Name)
	}
	if got := strings.Join(names, ","); got != "status,next_batch,resolve_entity,propose_entity,revise_entity,submit,propose_tasks,submit_tasks,record_decision,list_decisions,report" {
		t.Errorf("tools = %s", got)
	}

	var batch annotate.Batch
	if !c.tool("next_batch", map[string]any{"guide": "oziris-main", "section": "1.1", "limit": 5}, &batch) {
		t.FailNow()
	}
	if len(batch.Steps) != 5 || batch.Steps[4].StepID != "1.1.5" || batch.Steps[4].Text != "Buy a spade, start X marks the spot quest" {
		t.Fatalf("batch = %+v", batch.Steps)
	}
	var axe annotate.EntityResult
	if !c.tool("resolve_entity", map[string]any{"kind": "item", "name": "axe"}, &axe) || axe.Status != "ambiguous" || len(axe.Candidates) < 2 {
		t.Errorf("axe = %+v", axe)
	}

	var res annotate.SubmitResult
	ok := c.tool("submit", map[string]any{
		"guide": "oziris-main", "section": "1.1", "skill_version": "annotate-guide@1",
		"steps": []map[string]any{
			{"step_id": "1.1.3", "comment": "account security advice, no objective"},
			{"step_id": "1.1.5", "objectives": []map[string]any{
				{"role": "starts", "kind": "quest", "name": "X Marks the Spot", "state": "started", "quote": "start X marks the spot quest", "confidence": "high"},
				{"role": "uses", "kind": "item", "name": "spade", "n": 1, "quote": "Buy a spade", "confidence": "high"},
			}},
			{"step_id": "1.1.4", "objectives": []map[string]any{
				{"role": "completes", "kind": "quest", "name": "Not a quest", "state": "finished", "quote": "x", "confidence": "high"},
			}},
		},
	}, &res)
	if !ok || len(res.Accepted) != 2 || len(res.Rejected) != 1 || res.Rejected[0].StepID != "1.1.4" {
		t.Fatalf("submit = %+v", res)
	}
	// Tasks (docs/10 section 7): the step as its tasks see it, then a task
	// from the text through submit_tasks, which keeps the objectives.
	var view annotate.TaskProposals
	if !c.tool("propose_tasks", map[string]any{"guide": "oziris-main", "step": "1.1.5"}, &view) ||
		!view.Annotated || len(view.Goal) != 1 || view.Goal[0] != "quest:x_marks_the_spot@started" || len(view.Proposals) != 0 {
		t.Fatalf("propose_tasks = %+v", view)
	}
	ok = c.tool("submit_tasks", map[string]any{
		"guide": "oziris-main", "section": "1.1", "skill_version": "annotate-guide@3",
		"steps": []map[string]any{{"step_id": "1.1.5", "tasks": []map[string]any{
			{"type": "buy", "shop": map[string]any{"name": "Lumbridge General Store"}, "item": map[string]any{"name": "Spade"}, "qty": 1, "quote": "Buy a spade", "confidence": "high"},
			{"type": "talk", "npc": map[string]any{"name": "Veos"}, "until": map[string]any{"objective": map[string]any{"kind": "quest", "name": "X Marks the Spot", "state": "started"}},
				"quote": "start X marks the spot quest", "confidence": "medium", "confirmed_by": "owner"},
		}}},
	}, &res)
	if !ok || len(res.Accepted) != 1 || len(res.Accepted[0].Tasks) != 2 {
		t.Fatalf("submit_tasks = %+v", res)
	}
	if !c.tool("submit", map[string]any{"guide": "oziris-main", "section": "1.1", "skill_version": "annotate-guide@1", "steps": []map[string]any{{"step_id": "1.1.5", "bogus": 1}}}, &res) {
		t.Log("unknown field refused, as expected")
	} else {
		t.Error("unknown argument fields must be refused")
	}

	inW.Close()
	if serveErr := <-done; serveErr != nil {
		t.Fatal(serveErr)
	}

	loaded, err := content.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := loaded.StepInGuide("oziris-main", "1.1.5")
	var keys []string
	for _, o := range st.Objectives {
		keys = append(keys, o.Key.String()+" "+o.Role+" "+o.Source.Method)
	}
	// The rule's objective comes first; the annotation replaces it in place
	// (same key and role) and adds the spade.
	if got := strings.Join(keys, "|"); got != "quest:x_marks_the_spot@started starts llm|item:952x1 uses llm" {
		t.Errorf("composed 1.1.5 = %s", got)
	}
	if len(st.Tasks) != 2 || st.Tasks[0].Shop.String() != "shop:lumbridge_general_store" || st.Tasks[1].Until.Objective.String() != "quest:x_marks_the_spot@started" {
		t.Errorf("1.1.5 tasks = %+v", st.Tasks)
	}
	if _, err := os.Stat(filepath.Join(dir, "guides", "oziris-main", annotate.ReportFileName)); err != nil {
		t.Errorf("report not written: %v", err)
	}
}
