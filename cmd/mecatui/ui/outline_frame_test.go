package ui

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stacklok/mecatl/cmd/mecatui/client"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/scrollback"
)

func TestOutlineFrameLiveGrammar(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(62)
	m.phase = phaseRunning
	m.conv.addUser("check")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
	before := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if strings.Contains(ansi.Strip(strings.Join(before.lines, "\n")), "╭") {
		t.Fatalf("turn.start opened before visible content: %q", before.lines)
	}
	m = applyAll(m, client.TurnEndMsg{Turn: 1, Usage: client.Usage{InputTokens: 2}}, client.ToolCallMsg{ID: "edit", Name: "Edit", Args: `{"path":"foo.md"}`})
	const pendingFlatWant = " ▌ check\n\n ╭─────────────────────────────────────────────────────────╮\n │ … running · Edit                                        │\n │ path: \"foo.md\"                                          │\n ╰─────────────────────────────────────────────────────────╯\n"
	if pendingFlat := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n")); pendingFlat != pendingFlatWant {
		t.Fatalf("collapsed pending tool changed:\ngot  %q\nwant %q", pendingFlat, pendingFlatWant)
	}
	text := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(text, "╭◆─ ↑2 ↓0") || !strings.Contains(text, "├●╮ … running") || !strings.Contains(text, "Edit · 1s") {
		t.Fatalf("textless pending tool: %q", text)
	}
	m = applyAll(m, client.ToolResultMsg{CallID: "edit", Content: "done"}, client.CompactionMsg{Text: "compacted"})
	text = ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(text, "├●─ ✓ Edit · 1s · foo.md") || !strings.Contains(text, "│ ") {
		t.Fatalf("settled tool and notice: %q", text)
	}
	notice := false
	active := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	for i, row := range active.provenance {
		if row.kind == scrollback.KindNotice {
			notice = true
			if !strings.HasPrefix(ansi.Strip(active.lines[i]), "│ ") {
				t.Fatalf("notice missing outer rail: %q", active.lines[i])
			}
		}
	}
	if !notice {
		t.Fatal("compaction notice missing")
	}
	flatBoundary := false
	for i, row := range active.provenance {
		if row.separator {
			if active.lines[i] != "" || row.blockID != 0 || row.text {
				t.Fatalf("non-prompt structural separator survived: %q %+v", active.lines[i], row)
			}
			flatBoundary = true
		}
	}
	if !flatBoundary {
		t.Fatalf("missing flat prompt boundary: %q", active.lines)
	}
	m = applyAll(m, client.TurnStartMsg{Turn: 2})
	if openSecond := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n")); strings.Contains(openSecond, "├◆╮") {
		t.Fatalf("second turn opened before delta: %q", openSecond)
	}
	m = applyAll(m, client.AssistantDeltaMsg{Turn: 2, Text: "answer"}, client.TurnEndMsg{Turn: 2, DurationMs: 7000}, client.ResultMsg{Stop: "end_turn", Usage: client.Usage{InputTokens: 100, OutputTokens: 5, CacheReadTokens: 100}})
	flat := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n"))
	// Glamour may right-pad a prose row; only that non-semantic padding is removed.
	flatRows := strings.Split(flat, "\n")
	for i := range flatRows {
		flatRows[i] = strings.TrimRight(flatRows[i], " ")
	}
	flat = strings.Join(flatRows, "\n")
	const flatWant = " ▌ check\n\n ✓ Edit · foo.md\n • history compacted: compacted\n\n ◆  answer\n"
	if flat != flatWant {
		t.Fatalf("collapsed transcript changed:\ngot  %q\nwant %q", flat, flatWant)
	}
	if strings.Contains(flat, "↑2 ↓0") || strings.Contains(flat, "╭") || strings.Contains(flat, "╰─") || !strings.Contains(flat, "answer") || !strings.Contains(flat, "foo.md") || !strings.Contains(flat, "history compacted") {
		t.Fatalf("collapsed content/visibility drift: %q", flat)
	}
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	text = ansi.Strip(strings.Join(frame.lines, "\n"))
	for _, want := range []string{"├◆╮", "│ ╰─ ↑", "╰─ ↑100 ↓5 · LLM 7s · tools 1s · 100% cached"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %q", want, text)
		}
	}
	if len(frame.lines) != len(frame.provenance) {
		t.Fatal("provenance not lockstep")
	}
	if r := m.rend.th.Style("runRail").Render("│"); r == m.rend.th.Style("turnRail").Render("│") {
		t.Fatal("rail styles identical")
	}
	for _, line := range frame.lines {
		if strings.HasPrefix(ansi.Strip(line), "├◆╮") && !strings.HasPrefix(line, m.rend.outlineGlyph("├◆╮")) {
			t.Fatalf("styled glyph roles missing: %q", line)
		}
	}
	for _, row := range frame.provenance {
		if row.blockID != 0 && row.region == conversationRegionBody && row.kind == scrollback.KindAssistant && !row.text {
			t.Fatal("lost assistant body provenance")
		}
	}
}

func TestOutlineFrameOneBoundaryPerCompletedTurn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		visible []bool
		glyphs  []string
	}{
		{"visible then textless", []bool{true, false, false}, []string{"│ ╰─", "├◆─", "├◆─"}},
		{"first textless", []bool{false, false, true}, []string{"╭◆─", "├◆─", "│ ╰─"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := outlineTestModel()
			m.rend.setWidth(90)
			m.phase = phaseRunning
			m.conv.addUser("prompt")
			m = applyAll(m, client.SessionInitMsg{})
			for i, visible := range tc.visible {
				m = applyAll(m, client.TurnStartMsg{Turn: int32(i + 1)})
				if visible {
					m = applyAll(m, client.AssistantDeltaMsg{Turn: int32(i + 1), Text: "answer"})
				}
				m = applyAll(m, client.TurnEndMsg{Turn: int32(i + 1)})
			}
			m = applyAll(m, client.ResultMsg{Stop: "end_turn"})
			run := m.conv.scrollback.Outline().Runs()[0]
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			boundaries := 0
			for i, turn := range run.Turns {
				statID := scrollback.BlockID(0)
				for j := 0; j < m.conv.scrollback.Len(); j++ {
					meta := m.conv.scrollback.MetadataAt(j)
					if role, ok := m.conv.scrollback.Outline().Card(meta.ID); ok && role.TurnID == turn.ID && meta.Kind == scrollback.KindTurnStat {
						statID = meta.ID
					}
				}
				if statID == 0 || !turn.Ended {
					t.Fatalf("completed turn %d lacks stat identity: %+v", i, turn)
				}
				count := 0
				for j, row := range frame.provenance {
					if row.blockID == uint64(statID) {
						got := ansi.Strip(frame.lines[j])
						if strings.HasPrefix(got, "╰─") {
							continue
						} // run footer shares its last card's stable ID
						count++
						if !strings.HasPrefix(got, tc.glyphs[i]+" ") || !strings.Contains(got, "↑0 ↓0") || (!turn.Visible && !strings.Contains(got, "◆")) {
							t.Fatalf("turn %d boundary = %q, want %s", i, got, tc.glyphs[i])
						}
					}
				}
				if count != 1 {
					t.Fatalf("turn %d has %d boundary rows, want one", i, count)
				}
				boundaries += count
			}
			if boundaries != len(tc.visible) {
				t.Fatalf("boundaries = %d, turns = %d", boundaries, len(tc.visible))
			}
		})
	}
}

func TestOutlineFrameTwoSyntheticToolBatches(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(90)
	m.phase = phaseRunning
	m.conv.addUser("inspect")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1},
		client.ToolCallMsg{ID: "read", Name: "Read", Args: `{"path":"foo.md"}`},
		client.ToolCallMsg{ID: "grep", Name: "Grep", Args: `{"pattern":"bar"}`})
	pending := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	for _, want := range []string{"Read · 1s", "Grep · 1.5s"} {
		if !strings.Contains(pending, want) {
			t.Fatalf("pending tool duration %q missing: %q", want, pending)
		}
	}
	m = applyAll(m, client.ToolResultMsg{CallID: "read", Content: "read"}, client.ToolResultMsg{CallID: "grep", Content: "found"})
	settled := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	for _, want := range []string{"✓ Read · 1s", "✓ Grep · 1.5s"} {
		if !strings.Contains(settled, want) {
			t.Fatalf("settled duration %q changed: %q", want, settled)
		}
	}
	m = applyAll(m, client.TurnStartMsg{Turn: 2}, client.TurnEndMsg{Turn: 2}, client.ToolCallMsg{ID: "edit", Name: "Edit", Args: `{"path":"baz.md"}`})
	if pending = ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n")); !strings.Contains(pending, "Edit · 2s") {
		t.Fatalf("second batch pending duration missing: %q", pending)
	}
	m = applyAll(m, client.ToolResultMsg{CallID: "edit", Content: "edited"}, client.ResultMsg{Stop: "end_turn", Usage: client.Usage{InputTokens: 9}})
	final := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(final, "✓ Edit · 2s") || !strings.Contains(final, "╰─ ↑9 ↓0 · tools 3.5s") {
		t.Fatalf("max-within-batch then sum changed: %q", final)
	}
	if got := m.conv.scrollback.Outline().Runs()[0].ToolDurationMs; got != 3500 {
		t.Fatalf("tool aggregate = %dms", got)
	}
}

func TestOutlineFrameSelectionSourceAcrossToggleAndResize(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m.rend.setWidth(65)
	m.conv.addUser("prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "Alpha βeta\n\n" + strings.Repeat("following content ", 6)})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	cardID := uint64(m.conv.scrollback.MetadataAt(1).ID)
	row := frame.firstRegionRow(cardID, conversationRegionBody)
	text, leading := selectionRowText(frame, frame.provenance[row], frame.lines[row])
	if !strings.HasPrefix(text, "Alpha βeta") {
		t.Fatalf("outlined semantic body = %q", text)
	}
	sel := selection{active: true, anchorL: row, headL: row, anchorC: leading + 6, headC: leading + 10}
	if !sel.snapshotLogical(frame, strings.Join(frame.lines, "\n")) || sel.copied != "βeta" {
		t.Fatalf("outlined selection = %+v", sel)
	}
	if point, ok := selectionPointFor(frame, row, leading+6); !ok || point.sourceOffset != 6 || !strings.HasPrefix(point.after, "βeta") {
		t.Fatalf("outlined source point = %+v, %v", point, ok)
	}
	collapsed := m.rend.renderConversationFrame(&m.conv.scrollback, false)
	if !sel.resolveLogical(collapsed) || sel.copied != "βeta" {
		t.Fatalf("F9 toggle changed selected text: %+v", sel)
	}
	m.rend.setWidth(30)
	reflowed := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if !sel.resolveLogical(reflowed) || sel.copied != "βeta" {
		t.Fatalf("width reflow changed selected text: %+v", sel)
	}
	m = applyAll(m, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "edit", Name: "Edit", Args: `{"path":"foo.md"}`}, client.ToolResultMsg{CallID: "edit", Content: "done"})
	toolFrame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	toolID := uint64(m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID)
	toolRow := toolFrame.firstRegionRow(toolID, conversationRegionBody)
	toolText, toolLeading := selectionRowText(toolFrame, toolFrame.provenance[toolRow], toolFrame.lines[toolRow])
	pathAt := strings.Index(toolText, "foo.md")
	if pathAt < 0 {
		t.Fatalf("tool semantic text lost path: %q", toolText)
	}
	pathCol := toolLeading + graphemeCount(toolText[:pathAt])
	toolSel := selection{active: true, anchorL: toolRow, headL: toolRow, anchorC: pathCol, headC: pathCol + graphemeCount("foo.md")}
	if !toolSel.snapshotLogical(toolFrame, strings.Join(toolFrame.lines, "\n")) || toolSel.copied != "foo.md" {
		t.Fatalf("outer rail shifted tool selection: %+v", toolSel)
	}
	toolSourceOffset := toolSel.anchorPoint.sourceOffset
	if want := graphemeCount("✓ Edit · "); toolSourceOffset != want {
		t.Fatalf("tool canonical path offset = %d, want %d", toolSourceOffset, want)
	}
	collapsedTool := m.rend.renderConversationFrame(&m.conv.scrollback, false)
	if !toolSel.resolveLogical(collapsedTool) || toolSel.copied != "foo.md" {
		t.Fatalf("collapsed tool selection shifted: %+v (offset %d)", toolSel, toolSourceOffset)
	}
	if point, ok := selectionPointFor(collapsedTool, toolSel.anchorL, toolSel.anchorC); !ok || point.sourceOffset != toolSourceOffset || !strings.HasPrefix(point.after, "foo.md") {
		t.Fatalf("collapsed canonical point = %+v, %v", point, ok)
	}
	m.rend.setWidth(65)
	wideTool := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if !toolSel.resolveLogical(wideTool) || toolSel.copied != "foo.md" {
		t.Fatalf("wide tool selection shifted: %+v (offset %d)", toolSel, toolSourceOffset)
	}
	if point, ok := selectionPointFor(wideTool, toolSel.anchorL, toolSel.anchorC); !ok || point.sourceOffset != toolSourceOffset || !strings.HasPrefix(point.after, "foo.md") {
		t.Fatalf("wide canonical point = %+v, %v", point, ok)
	}
}

func TestOutlineModelToggleResizeRetainsReadingAndSelection(t *testing.T) {
	m := outlineTestModel()
	m = applyAll(m, tea.WindowSizeMsg{Width: 70, Height: 12})
	m.phase = phaseRunning
	m.conv.addUser("prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "Alpha βeta\n\n" + strings.Repeat("following content ", 6)}, client.TurnEndMsg{Turn: 1})
	for range 15 {
		m = applyAll(m, client.CompactionMsg{Text: "later activity"})
	}
	m.refreshView()
	cardID := uint64(m.conv.scrollback.MetadataAt(1).ID)
	row := m.conversationView.frame.firstRegionRow(cardID, conversationRegionBody)
	if row < 0 {
		t.Fatal("assistant body missing")
	}
	m.vp.SetHeight(4)
	m.vp.SetYOffset(row)
	m.conversationView.mode = anchored
	m.conversationView.observe(m.vp)
	if m.conversationView.anchor.blockID != cardID {
		t.Fatalf("reading anchor before toggle = %+v", m.conversationView.anchor)
	}
	plain := ansi.Strip(strings.Split(m.vp.GetContent(), "\n")[row])
	at := strings.Index(plain, "βeta")
	if at < 0 {
		t.Fatalf("selected word absent: %q", plain)
	}
	m = m.wordSelect(row, graphemeCount(plain[:at]))
	if !m.sel.active || m.sel.copied != "βeta" || m.sel.anchorPoint.blockID != cardID || m.sel.anchorPoint.sourceOffset != 6 {
		t.Fatalf("selection before toggle = %+v", m.sel)
	}
	for _, msg := range []tea.Msg{tea.KeyPressMsg{Code: tea.KeyF9}, tea.WindowSizeMsg{Width: 35, Height: 12}, tea.KeyPressMsg{Code: tea.KeyF9}} {
		m = applyAll(m, msg)
		if !m.sel.active || m.sel.copied != "βeta" || selectedText(m.vp.GetContent(), m.sel) != "βeta" || m.sel.anchorPoint.blockID != cardID || m.sel.anchorPoint.sourceOffset != 6 {
			t.Fatalf("toggle/resize changed selection after %T: %+v", msg, m.sel)
		}
		if m.conversationView.mode != anchored || m.conversationView.anchor.blockID != cardID || m.conversationView.frame.provenance[m.vp.YOffset()].blockID != cardID {
			t.Fatalf("toggle/resize moved reading anchor after %T: anchor=%+v offset=%d", msg, m.conversationView.anchor, m.vp.YOffset())
		}
	}
	m = applyAll(m, tea.KeyPressMsg{Code: tea.KeyF9})
	if !m.expandConversation || !m.sel.active || m.sel.copied != "βeta" {
		t.Fatalf("failed to restore active outline before completion: %+v", m.sel)
	}
	m = applyAll(m, client.ResultMsg{Stop: "end_turn", Usage: client.Usage{InputTokens: 21}})
	m.refreshView()
	if !m.sel.active || m.sel.copied != "βeta" || selectedText(m.vp.GetContent(), m.sel) != "βeta" || m.sel.anchorPoint.blockID != cardID || m.sel.anchorPoint.sourceOffset != 6 {
		t.Fatalf("single-turn collapse changed selection: %+v", m.sel)
	}
	if m.conversationView.mode != anchored || m.conversationView.anchor.blockID != cardID || m.conversationView.frame.provenance[m.vp.YOffset()].blockID != cardID {
		t.Fatalf("single-turn collapse moved reading anchor: %+v offset=%d", m.conversationView.anchor, m.vp.YOffset())
	}
	_, cmd := m.copySelection()
	if payload, ok := osc52Payload(collectLeaves(cmd)); !ok || payload != "βeta" {
		t.Fatalf("copy payload after toggle/resize = %q, %v", payload, ok)
	}
}

func TestOutlineFrameKeepsHistoricalSeparatorsFlat(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(80)
	m.conv = conversationFromTranscript([]client.ConversationMessage{{Role: "user", Text: "historical prompt"}, {Role: "assistant", Text: "historical reply"}})
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "live reply"})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	blank := 0
	for i, row := range frame.provenance {
		if row.separator {
			if frame.lines[i] != "" {
				t.Fatalf("historical separator acquired rail: %q", frame.lines[i])
			}
			blank++
		}
	}
	if blank == 0 {
		t.Fatal("no historical separator to verify")
	}
}

func TestOutlineTerminalErrorClosesAfterErrorCard(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		name := "transient"
		if permanent {
			name = "permanent"
		}
		t.Run(name, func(t *testing.T) {
			m := outlineTestModel()
			m.rend.setWidth(80)
			m.phase = phaseRunning
			m.conv.addUser("prompt")
			m = applyAll(m,
				client.SessionInitMsg{},
				client.TurnStartMsg{Turn: 1},
				client.AssistantDeltaMsg{Turn: 1, Text: "response"},
				client.TurnEndMsg{Turn: 1},
			)
			runID := m.outlineRun
			m = applyAll(m, client.ResultMsg{Stop: "error", Error: "terminal failure", Permanent: permanent, Usage: client.Usage{InputTokens: 9, OutputTokens: 2}})
			errorID := m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID
			if role, ok := m.conv.scrollback.Outline().Card(errorID); !ok || role.RunID != runID || role.TurnID != 0 {
				t.Fatalf("terminal error not associated with run: %+v, %v", role, ok)
			}
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			errorRow, footerRow := -1, -1
			for i, line := range frame.lines {
				plain := ansi.Strip(line)
				if frame.provenance[i].blockID == uint64(errorID) && strings.Contains(plain, "terminal failure") {
					errorRow = i
					if !strings.HasPrefix(plain, "│ ") {
						t.Fatalf("error missing outer rail: %q", plain)
					}
				}
				if strings.HasPrefix(plain, "╰─ ↑9 ↓2 · error") {
					footerRow = i
				}
			}
			if errorRow < 0 || footerRow <= errorRow {
				t.Fatalf("footer did not follow outlined error: %q", frame.lines)
			}
			if m.usage.InputTokens != 9 || m.usage.OutputTokens != 2 {
				t.Fatalf("authoritative usage changed: %+v", m.usage)
			}
		})
	}
}

func TestOutlineFrameApprovalNotice(t *testing.T) {
	m, _ := newQueueModel(t)
	m.conv.addNotice("unrelated history")
	historyID := m.conv.scrollback.MetadataAt(0).ID
	m = startRunning(t, m, "prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "shell", Name: "Shell", Args: `{}`}, client.PermissionAskMsg{AskID: "sess-test-0001:1:shell", Tool: "Shell"})
	runID := m.outlineRun
	m, _ = pressKey(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if role, ok := m.conv.scrollback.Outline().Card(historyID); ok {
		t.Fatalf("historical notice joined run: %+v", role)
	}
	card := m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1)
	if card.Kind != scrollback.KindNotice {
		t.Fatalf("approval notice card = %+v", card)
	}
	if role, ok := m.conv.scrollback.Outline().Card(card.ID); !ok || role.RunID != runID || role.TurnID != 0 {
		t.Fatalf("approval membership = %+v, %v", role, ok)
	}
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	found := false
	for i, row := range frame.provenance {
		if row.blockID == uint64(card.ID) && strings.Contains(ansi.Strip(frame.lines[i]), "permission allowed") {
			if !strings.HasPrefix(ansi.Strip(frame.lines[i]), "│ ") {
				t.Fatalf("approval notice missing outer rail: %q", frame.lines[i])
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("approval notice not rendered: %q", frame.lines)
	}
}

func TestOutlineFrameStreamErrorClosesAfterErrorCard(t *testing.T) {
	for _, clearing := range []bool{false, true} {
		name := "ordinary"
		if clearing {
			name = "clear pending"
		}
		t.Run(name, func(t *testing.T) {
			m := outlineTestModel()
			m.rend.setWidth(80)
			m.phase = phaseRunning
			m.conv.addUser("prompt")
			m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "response"}, client.TurnEndMsg{Turn: 1})
			runID := m.outlineRun
			if clearing {
				m.clearPending = &clearHandoff{sourceID: m.sessionID, sourcePhase: phaseRunning}
			}
			m = applyAll(m, client.StreamErrMsg{Err: errors.New("lost stream")})
			if got, _ := m.conv.scrollback.Outline().Run(runID); got.Status != scrollback.RunInterrupted {
				t.Fatalf("run status = %+v", got)
			}
			if clearing && (m.clearPending == nil || !m.clearPending.sourceSettled) {
				t.Fatalf("clear source not settled: %+v", m.clearPending)
			}
			card := m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1)
			if card.Kind != scrollback.KindError {
				t.Fatalf("last card = %+v, want error", card)
			}
			if role, ok := m.conv.scrollback.Outline().Card(card.ID); !ok || role.RunID != runID || role.TurnID != 0 {
				t.Fatalf("error membership = %+v, %v", role, ok)
			}
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			errorRow, footerRow := -1, -1
			for i, line := range frame.lines {
				plain := ansi.Strip(line)
				if frame.provenance[i].blockID == uint64(card.ID) && strings.Contains(plain, "lost stream") {
					errorRow = i
					if !strings.HasPrefix(plain, "│ ") {
						t.Fatalf("error row without outer rail: %q", plain)
					}
				}
				if strings.HasPrefix(plain, "╰─ interrupted") {
					footerRow = i
				}
			}
			if errorRow < 0 || footerRow <= errorRow {
				t.Fatalf("error/footer order: %q", frame.lines)
			}
		})
	}
}

func TestOutlineNoTurnInterruptionsAtSharedBoundary(t *testing.T) {
	for _, withHistory := range []bool{false, true} {
		m := outlineTestModel()
		m.rend.setWidth(80)
		if withHistory {
			m.conv = conversationFromTranscript([]client.ConversationMessage{{Role: "assistant", Text: "historical answer"}})
		}
		for _, terminal := range []tea.Msg{client.ResultMsg{Stop: "cancelled"}, client.StreamClosedMsg{}, client.ResultMsg{Stop: "cancelled"}} {
			m.phase = phaseRunning
			m = applyAll(m, client.SessionInitMsg{}, terminal)
		}
		m.conv.addUser("next prompt")
		m.phase = phaseRunning
		m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
		for _, reconnect := range []tea.Msg{reconnectMsg{gen: m.liveReconGen, msg: client.LiveReconnectingMsg{Attempt: 1}}, reconnectMsg{gen: m.liveReconGen, msg: client.LiveReconnectedMsg{}}} {
			m = applyAll(m, reconnect)
			plain := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
			next := strings.Index(plain, "next prompt")
			if next < 0 || strings.Count(plain[:next], "╰─ interrupted") != 3 || strings.Contains(plain, "◆") || strings.ContainsAny(plain[next:], "╭╰") || (withHistory && strings.Index(plain, "historical answer") > strings.Index(plain, "╰─")) {
				t.Fatalf("history=%t: no-turn closures/reconnect ordering:\n%s", withHistory, plain)
			}
		}
	}
}

func TestOutlineHiddenFinalCardClosurePreservesContent(t *testing.T) {
	m := outlineTestModel()
	m.expandConversation = true
	m.rend.setWidth(80)
	m.conv.addUser("first prompt")
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "Alpha βeta\n\n" + strings.Repeat("following content ", 6)}, client.TurnEndMsg{Turn: 1, DurationMs: 500}, client.TurnStartMsg{Turn: 2})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	cardID := uint64(m.conv.scrollback.MetadataAt(1).ID)
	row := frame.firstRegionRow(cardID, conversationRegionBody)
	_, leading := selectionRowText(frame, frame.provenance[row], frame.lines[row])
	sel := selection{active: true, anchorL: row, headL: row, anchorC: leading + 6, headC: leading + 10}
	if !sel.snapshotLogical(frame, strings.Join(frame.lines, "\n")) || sel.copied != "βeta" {
		t.Fatalf("initial selection: %+v", sel)
	}
	anchor, ok := frame.anchorForRow(row)
	if !ok {
		t.Fatal("initial anchor missing")
	}
	renders := m.rend.blockRenders
	m = applyAll(m, client.ResultMsg{Stop: "cancelled"})
	frame = m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if m.rend.blockRenders != renders {
		t.Fatal("outline-only closure rerendered cached content")
	}
	hiddenID := uint64(m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID)
	for i, line := range frame.lines {
		if strings.Contains(ansi.Strip(line), "no visible output") || strings.HasPrefix(ansi.Strip(line), "╰─") {
			p := frame.provenance[i]
			if p.blockID != hiddenID || p.region != conversationRegionChrome || p.text {
				t.Fatalf("fallback chrome lost boundary provenance: %+v", p)
			}
		}
	}
	m.conv.addUser("next prompt")
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "next answer"}, client.TurnEndMsg{Turn: 1}, client.ResultMsg{Stop: "end_turn"})
	for _, width := range []int{80, 24, 80} {
		m.rend.setWidth(width)
		for _, expand := range []bool{true, false, true} {
			frame = m.rend.renderConversationFrame(&m.conv.scrollback, expand)
			if !sel.resolveLogical(frame) || sel.copied != "βeta" {
				t.Fatalf("width=%d expand=%t selection: %+v", width, expand, sel)
			}
			if _, ok := frame.rowForAnchor(anchor); !ok {
				t.Fatalf("width=%d expand=%t lost reading anchor", width, expand)
			}
			plain := ansi.Strip(strings.Join(frame.lines, "\n"))
			if expand {
				footer, next := strings.Index(plain, "interrupted"), strings.Index(plain, "next prompt")
				if footer < 0 || next <= footer || strings.Count(plain, "interrupted") != 1 || strings.Count(plain, "no visible output") != 1 {
					t.Fatalf("width=%d closure crossed newer output:\n%s", width, plain)
				}
			} else if strings.Contains(plain, "interrupted") || strings.Contains(plain, "no visible output") {
				t.Fatalf("F9-off exposed outline footer: %q", plain)
			}
			renders := m.rend.blockRenders
			cached := m.rend.renderConversationFrame(&m.conv.scrollback, expand)
			if m.rend.blockRenders != renders || strings.Join(cached.lines, "\n") != strings.Join(frame.lines, "\n") || len(frame.lines) != len(frame.provenance) {
				t.Fatal("unchanged cached frame/provenance drifted")
			}
		}
	}
}

func TestOutlineFrameInterruptedFooterAndCache(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(70)
	m.phase = phaseRunning
	m.conv.addUser("work")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "hello"})
	collapsed := strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n")
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	initialRenders := m.rend.blockRenders
	m.conv.scrollback.Outline().Interrupt(m.outlineRun)
	terminal := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if m.rend.blockRenders != initialRenders {
		t.Fatal("terminal closure unnecessarily invalidated card layout")
	}
	if got := strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n"); got != collapsed {
		t.Fatal("terminal outline changed F9-off bytes")
	}
	if !strings.Contains(ansi.Strip(strings.Join(terminal.lines, "\n")), "╰─ interrupted") {
		t.Fatalf("interrupted footer: %q", terminal.lines)
	}
	if len(terminal.lines) == len(frame.lines) {
		t.Fatal("closure did not invalidate frame prefix")
	}
}

func TestOutlineFrameTerminalBeforeFirstDelta(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(60)
	m.phase = phaseRunning
	m.conv.addUser("pending")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.ResultMsg{Stop: "error", Usage: client.Usage{InputTokens: 4}})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	plain := ansi.Strip(strings.Join(frame.lines, "\n"))
	if !strings.Contains(plain, "╭◆─") || !strings.Contains(plain, "╰─ ↑4 ↓0 · error") || strings.Index(plain, "╭◆─") > strings.Index(plain, "╰─") {
		t.Fatalf("empty terminal turn ordering: %q", plain)
	}
}

func TestOutlineFrameResultWithoutTurn(t *testing.T) {
	for _, prompt := range []bool{false, true} {
		m := outlineTestModel()
		m.rend.setWidth(70)
		m.phase = phaseRunning
		if prompt {
			m.conv.addUser("prompt")
		}
		before := m.rend.renderConversationFrame(&m.conv.scrollback, true)
		want := strings.Join(before.lines, "\n")
		m = applyAll(m, client.SessionInitMsg{}, client.ResultMsg{Stop: "end_turn", Usage: client.Usage{InputTokens: 3}})
		frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
		if got := ansi.Strip(strings.Join(frame.lines, "\n")); strings.Contains(got, "╭") || strings.Contains(got, "╰─") || (prompt && !strings.Contains(got, " ▌│ prompt")) {
			t.Fatalf("prompt=%t: zero-turn run gained chrome: %q (previous %q)", prompt, got, want)
		}
	}
}

func TestOutlinePreOpenerActivityRemainsFlat(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m.rend.setWidth(55)
	m.conv.addUser("prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.CompactionMsg{Text: "before turn"}, client.TurnStartMsg{Turn: 1}, client.CompactionMsg{Text: "before delta"})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	plain := ansi.Strip(strings.Join(frame.lines, "\n"))
	if strings.ContainsAny(plain, "╭╰") || strings.Contains(plain, "◇") || strings.Contains(plain, "│ •") {
		t.Fatalf("pre-opener notices created run chrome: %q", plain)
	}
	for i, line := range frame.lines {
		if frame.provenance[i].kind == scrollback.KindNotice && (!strings.HasPrefix(ansi.Strip(line), " ") || frame.provenance[i].blockID == 0) {
			t.Fatalf("pre-opener notice lost flat content/provenance: %q %+v", line, frame.provenance[i])
		}
	}
	m = applyAll(m, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "read", Name: "Read", Args: `{"path":"a.go"}`})
	plain = ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(plain, "╭◆─") || !strings.Contains(plain, "├●╮ … running") {
		t.Fatalf("textless marker did not start outlined tool: %q", plain)
	}
}

func TestOutlineZeroTurnTerminalActivityRemainsFlat(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m.rend.setWidth(55)
	m.conv.addUser("prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.CompactionMsg{Text: "before error"}, client.ResultMsg{Stop: "error", Error: "failure"})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	plain := ansi.Strip(strings.Join(frame.lines, "\n"))
	if strings.ContainsAny(plain, "╭╰") || strings.Contains(plain, "◇") || !strings.Contains(plain, "failure") || !strings.Contains(plain, "before error") {
		t.Fatalf("zero-turn terminal activity gained outline or lost content: %q", plain)
	}
	for i, row := range frame.provenance {
		if (row.kind == scrollback.KindNotice || row.kind == scrollback.KindError) && (row.blockID == 0 || strings.HasPrefix(ansi.Strip(frame.lines[i]), "│")) {
			t.Fatalf("zero-turn activity lost flat provenance: %q %+v", frame.lines[i], row)
		}
	}
}

func TestOutlineFrameTransitionsAndWidth(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m.conv.addUser("inspect")
	m.rend.setWidth(40)
	collapsedBefore := strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n")
	m = applyAll(m, client.SessionInitMsg{})
	if got := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n")); got != " ▌ inspect\n" {
		t.Fatalf("prompt actor on run open: %q (previous %q)", got, collapsedBefore)
	}
	open := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if strings.Contains(open, "╭") {
		t.Fatalf("run opened before visible activity: %q", open)
	}
	m = applyAll(m, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: strings.Repeat("界🌍 words ", 15)})
	first := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if !strings.Contains(ansi.Strip(strings.Join(first.lines, "\n")), "╭◆╮") {
		t.Fatalf("first visible turn: %q", first.lines)
	}
	for _, row := range first.lines {
		if ansi.StringWidth(row) > 40 {
			t.Fatalf("full width overflow: %q", row)
		}
	}
	id := m.conv.scrollback.MetadataAt(1).ID
	anchorRow := first.firstRegionRow(uint64(id), conversationRegionBody)
	anchor, ok := first.anchorForRow(anchorRow)
	if !ok || !anchor.text {
		t.Fatalf("missing text anchor: %+v", anchor)
	}
	if point, ok := selectionPointFor(first, anchorRow, 4); !ok || strings.HasPrefix(point.after, "╭") || strings.HasPrefix(point.after, "│") {
		t.Fatalf("rail entered selected source: %+v, %v", point, ok)
	}
	collapsed := m.rend.renderConversationFrame(&m.conv.scrollback, false)
	if _, ok := collapsed.rowForAnchor(anchor); !ok {
		t.Fatal("F9 lost content anchor")
	}
	m = applyAll(m, client.TurnEndMsg{Turn: 1}, client.TurnStartMsg{Turn: 2}, client.TurnEndMsg{Turn: 2}, client.ResultMsg{Stop: "error", Usage: client.Usage{InputTokens: 40}})
	m.rend.setWidth(16)
	narrow := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	plain := ansi.Strip(strings.Join(narrow.lines, "\n"))
	for _, want := range []string{"│ ╰─", "├◆─", "error"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in %q", want, plain)
		}
	}
	for _, row := range narrow.lines {
		if ansi.StringWidth(row) > 16 {
			t.Errorf("narrow overflow: %q", row)
		}
	}
	if _, ok := narrow.rowForAnchor(anchor); !ok {
		t.Fatal("reflow lost anchor")
	}
	for _, width := range []int{1, 2, 3, 4, 5} {
		m.rend.setWidth(width)
		frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
		if width == 5 && !strings.Contains(ansi.Strip(strings.Join(frame.lines, "\n")), "界") {
			t.Fatal("tiny terminal dropped assistant content")
		}
		for i, row := range frame.lines {
			if frame.provenance[i].kind == scrollback.KindUser {
				continue
			} // unchanged flat prompt
			if ansi.StringWidth(row) > width {
				t.Fatalf("width %d overflow: %q", width, row)
			}
		}
	}
}

func TestOutlineSingleTurnTerminalStable(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		terminal     client.ResultMsg
	}{
		{"normal", "", client.ResultMsg{Stop: "end_turn", Usage: client.Usage{InputTokens: 42, OutputTokens: 3}}},
		{"error", "error", client.ResultMsg{Stop: "error", Error: "failed", Usage: client.Usage{InputTokens: 42, OutputTokens: 3}}},
		{"interrupted", "interrupted", client.ResultMsg{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := outlineTestModel()
			m.rend.setWidth(35)
			m.phase = phaseRunning
			m.conv.addUser("prompt")
			m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "Alpha βeta\n\nlast paragraph"})
			active := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			if !strings.Contains(ansi.Strip(strings.Join(active.lines, "\n")), "╭◆╮") {
				t.Fatalf("active run not nested: %q", active.lines)
			}
			assertBodyBlank := func(stage string, frame renderedFrame, id uint64) {
				t.Helper()
				start, end := -1, -1
				for i, line := range frame.lines {
					if frame.provenance[i].blockID != id || frame.provenance[i].region != conversationRegionBody {
						continue
					}
					plain := ansi.Strip(line)
					if strings.Contains(plain, "Alpha βeta") {
						start = i
					}
					if strings.Contains(plain, "last paragraph") {
						end = i
					}
				}
				if start < 0 || end != start+2 || strings.TrimSpace(strings.TrimLeft(ansi.Strip(frame.lines[start+1]), "│╭ ")) != "" {
					t.Fatalf("%s: assistant model blank row missing: %q", stage, frame.lines)
				}
			}
			id := uint64(m.conv.scrollback.MetadataAt(1).ID)
			assertBodyBlank("active", active, id)
			row := active.firstRegionRow(id, conversationRegionBody)
			anchor, ok := active.anchorForRow(row)
			if !ok || !anchor.text {
				t.Fatalf("body anchor missing: %+v", anchor)
			}
			m = applyAll(m, client.TurnEndMsg{Turn: 1, DurationMs: 800})
			if tc.name == "interrupted" {
				m = applyAll(m, client.StreamErrMsg{Err: errors.New("disconnected")})
			} else {
				m = applyAll(m, tc.terminal)
			}
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			assertBodyBlank("terminal", frame, id)
			plain := ansi.Strip(strings.Join(frame.lines, "\n"))
			if !strings.Contains(plain, "╭◆╮") || !strings.Contains(plain, "│ │") || !strings.Contains(plain, "│ ╰─") || !strings.Contains(plain, "╰─") || !strings.Contains(plain, tc.status) || strings.Count(plain, "◆") != 1 {
				t.Fatalf("single-turn %s grammar changed: %q", tc.name, plain)
			}
			for i, line := range active.lines {
				if active.provenance[i].blockID == id {
					if i >= len(frame.lines) || frame.lines[i] != line {
						t.Fatalf("terminal reflowed existing content row %d", i)
					}
				}
			}
			if tc.name != "interrupted" && !strings.Contains(plain, "↑42 ↓3 · LLM 800ms") {
				t.Fatalf("run totals lost: %q", plain)
			}
			if target, ok := frame.rowForAnchor(anchor); !ok || frame.provenance[target].blockID != id || !frame.provenance[target].text {
				t.Fatalf("collapse lost body anchor: %d %v", target, ok)
			}
			for _, line := range frame.lines {
				if ansi.StringWidth(line) > 35 {
					t.Fatalf("terminal overflow: %q", line)
				}
			}
		})
	}
}

func TestOutlineTextlessCompletedSingleTurn(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(45)
	m.phase = phaseRunning
	m.conv.addUser("prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
	if got := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n")); strings.Contains(got, "╭") {
		t.Fatalf("turn.start alone opened a rail: %q", got)
	}
	m = applyAll(m, client.TurnEndMsg{Turn: 1, Usage: client.Usage{InputTokens: 6}, DurationMs: 400})
	active := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(active, "╭◆─ ↑6 ↓0 · 400ms") {
		t.Fatalf("textless completed turn missing semantic marker: %q", active)
	}
	m = applyAll(m, client.ResultMsg{Stop: "end_turn", Usage: client.Usage{InputTokens: 6}})
	final := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(final, "╭◆─ ↑6 ↓0 · 400ms") || !strings.Contains(final, "╰─ ↑6 ↓0 · LLM 400ms") {
		t.Fatalf("textless terminal turn changed: %q", final)
	}
}

func TestOutlineNarrowExpandedReasoningWidth(t *testing.T) {
	for _, width := range []int{12, 16, 20} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			m := outlineTestModel()
			m.phase = phaseRunning
			m.rend.setWidth(width)
			m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.ReasoningDeltaMsg{Turn: 1, Text: "a reason"}, client.AssistantDeltaMsg{Turn: 1, Text: "answer"})
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			id := uint64(m.conv.scrollback.MetadataAt(0).ID)
			bodyRow := frame.firstRegionRow(id, conversationRegionReasoning)
			if bodyRow < 0 || !frame.provenance[bodyRow].text || frame.provenance[bodyRow].sourceOffset != 0 {
				t.Fatalf("width %d: reasoning provenance: %+v", width, frame.provenance)
			}
			if point, ok := selectionPointFor(frame, bodyRow, frame.provenance[bodyRow].rail); !ok || point.region != conversationRegionReasoning || point.sourceOffset != 0 || !strings.HasPrefix(point.after, "a") {
				t.Fatalf("width %d: first reasoning text not selectable from offset zero: %+v %v", width, point, ok)
			}
			if anchor, ok := frame.anchorForRow(bodyRow); !ok || !anchor.text {
				t.Fatalf("width %d: first reasoning text has no anchor: %+v %v", width, anchor, ok)
			}
			if got := ansi.Strip(strings.Join(frame.lines, "\n")); strings.Count(got, "◆") != 1 || strings.Contains(got, "f9") {
				t.Fatalf("call marker/chrome at width %d: %q", width, got)
			}
			for i, line := range frame.lines {
				if w := ansi.StringWidth(line); w > width {
					t.Fatalf("width %d: outlined row %d uses %d columns: %q", width, i, w, line)
				}
			}
		})
	}
}

func TestOutlineReasoningOnlyAndPackedStructure(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(70)
	m.phase = phaseRunning
	m.conv.addUser("prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
	before := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if strings.Contains(ansi.Strip(strings.Join(before.lines, "\n")), "╭") {
		t.Fatalf("start opened before reasoning: %q", before.lines)
	}
	m = applyAll(m, client.ReasoningDeltaMsg{Turn: 1, Text: "step one\n\nstep two"})
	active := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(active, "╭◆╮") || !strings.Contains(active, "╭◆╮ step one") {
		t.Fatalf("reasoning without answer did not open: %q", active)
	}
	m = applyAll(m, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "read", Name: "Read", Args: `{"path":"a.go"}`}, client.ToolResultMsg{CallID: "read", Content: "ok"}, client.CompactionMsg{Text: "compact"}, client.TurnStartMsg{Turn: 2})
	pending := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if strings.Contains(ansi.Strip(strings.Join(pending.lines, "\n")), "├◆╮") {
		t.Fatalf("empty second turn opened: %q", pending.lines)
	}
	m = applyAll(m, client.TurnEndMsg{Turn: 2}, client.ResultMsg{Stop: "end_turn"})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	plain := ansi.Strip(strings.Join(frame.lines, "\n"))
	if !strings.Contains(plain, "├◆─") || !strings.Contains(plain, "╭◆╮") || strings.Count(plain, "◆") != 2 {
		t.Fatalf("multi-turn boundaries lost: %q", plain)
	}
	for i, row := range frame.provenance {
		if row.separator && i > 2 {
			t.Fatalf("structural gap at %d: %q", i, frame.lines)
		}
	}
	if !strings.Contains(plain, "╭◆╮ step one\n│ │         \n│ │ step two") {
		t.Fatalf("reasoning blank line lost: %q", plain)
	}
}
