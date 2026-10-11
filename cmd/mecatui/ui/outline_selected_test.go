package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stacklok/mecatl/cmd/mecatui/client"
	"github.com/stacklok/mecatl/cmd/mecatui/theme"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/scrollback"
)

func TestSelectedOutlineGrammar(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(100)
	m.phase = phaseRunning
	m.conv.addUser("Prompt\ncontinuation")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.ReasoningDeltaMsg{Turn: 1, Text: "thinking"}, client.AssistantDeltaMsg{Turn: 1, Text: "answer"}, client.TurnEndMsg{Turn: 1, DurationMs: 1000}, client.ToolCallMsg{ID: "a", Name: "Read", Args: `{"path":"a.go"}`}, client.ToolResultMsg{CallID: "a", Content: "ok"})
	single := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(single, "├●─ ✓ Read") {
		t.Fatalf("singleton: %q", single)
	}
	m = applyAll(m, client.ToolCallMsg{ID: "b", Name: "Read", Args: `{"path":"b.go"}`}, client.ToolResultMsg{CallID: "b", Content: "ok"}, client.ToolCallMsg{ID: "c", Name: "Read", Args: `{"path":"c.go"}`}, client.ToolResultMsg{CallID: "c", Content: "ok", IsError: true}, client.TurnStartMsg{Turn: 2}, client.TurnEndMsg{Turn: 2, Usage: client.Usage{InputTokens: 14000, OutputTokens: 80, CacheReadTokens: 13720}, DurationMs: 1000})
	m.steer = &steerState{Sends: []steerQueuedSend{{ID: "delivered", Text: "ordinary prompt"}}}
	m = applyAll(m, client.SteerEchoMsg{MessageID: "delivered", Text: "ordinary prompt\nsecond line"}, client.TurnStartMsg{Turn: 3}, client.AssistantDeltaMsg{Turn: 3, Text: "final answer"}, client.TurnEndMsg{Turn: 3}, client.ResultMsg{Stop: "end_turn"})
	got := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	for _, want := range []string{" ▌│ Prompt\n  │ continuation", "╭◆╮ thinking", "│ │ answer", "│ ╰─ ↑", "├●┬ ✓ Read", "│ ├ ✓ Read", "│ ╰ ✗ Read", "├◆─ ↑", "├▌│ ordinary prompt\n│ │ second line", "├◆╮ final answer", "╰─ ↑"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "◆") != 3 || strings.Count(got, "●") != 1 || strings.Contains(got, "reasoning ·") {
		t.Fatalf("actor counts/chrome: %q", got)
	}
	rows := strings.Split(got, "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	const wantTranscript = " ▌│ Prompt\n  │ continuation\n\n╭◆╮ thinking\n│ │ answer\n│ ╰─ ↑0 ↓0 · 1s\n├●┬ ✓ Read · 1s · a.go\n│ ├ ✓ Read · 1.5s · b.go\n│ ╰ ✗ Read · 2s · c.go · failed\n├◆─ ↑14K ↓80 · 1s · 98% cached\n├▌│ ordinary prompt\n│ │ second line\n├◆╮ final answer\n│ ╰─ ↑0 ↓0\n╰─ ↑0 ↓0 · LLM 2s · tools 2s\n"
	if strings.Join(rows, "\n") != wantTranscript {
		t.Fatalf("combined transcript:\ngot  %q\nwant %q", strings.Join(rows, "\n"), wantTranscript)
	}
	for _, tool := range m.conv.scrollback.Outline().Runs()[0].Tools {
		role, _ := m.conv.scrollback.Outline().Card(tool.CardID)
		if tool.BatchID == 0 || !role.Tool || role.RunID != m.conv.scrollback.Outline().Runs()[0].ID {
			t.Fatalf("stored membership not exposed: %+v %+v", role, tool)
		}
	}
	mono := newRenderer(theme.New("monochrome", theme.Palette{}), m.rend.marks)
	mono.setWidth(100)
	monoRows := mono.renderConversationFrame(&m.conv.scrollback, true).lines
	for i := range monoRows {
		monoRows[i] = strings.TrimRight(ansi.Strip(monoRows[i]), " ")
	}
	if strings.Join(monoRows, "\n") != wantTranscript {
		t.Fatalf("monochrome grammar: %q", monoRows)
	}
	m.rend.setWidth(12)
	narrow := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if strings.Count(narrow, "◆") != 3 || strings.Count(narrow, "●") != 1 || strings.Count(narrow, "│ ╰─") != 2 {
		t.Fatalf("wrapped stats repeated call markers or footers: %q", narrow)
	}
}

func TestSelectedOutlineDrainEchoOwnsPromptWithMedia(t *testing.T) {
	m, _ := newSteerModel(t, true)
	m = startRunning(t, m, "initial prompt")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "working"})
	runID := m.outlineRun
	m.caps.Image = true
	m.prompt.Rewrite("delivered words [Image #1]")
	m.stagedMedia = map[string]stagedAttachment{"[Image #1]": {mime: "image/png", data: []byte("pixels")}}
	mm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mm.(Model)
	runBatchLeaves(cmd)
	m = enqueueSteer(t, m, "still pending")
	m = applyAll(m, client.SteerOutcomeMsg{Outcome: client.SteerAccepted, MessageID: "steer-0001"}, client.TurnEndMsg{Turn: 1})
	if got := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n")); strings.Contains(got, "delivered words") || strings.Contains(got, "still pending") || m.renderSteer() == "" {
		t.Fatalf("local boundary inferred delivery: %q", got)
	}
	echo := client.SteerEchoMsg{MessageID: "steer-0001", Text: "delivered words\ncontinuation", Parts: []client.ContentBlock{{Kind: client.ContentBlockImage, MimeType: "image/png", Data: []byte("pixels")}}}
	m = applyAll(m, echo)
	id := m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID
	role, ok := m.conv.scrollback.Outline().Card(id)
	if !ok || role.RunID != runID || role.InitialPrompt || m.steer == nil || len(m.steer.Sends) != 1 || m.steer.Sends[0].Text != "still pending" {
		t.Fatalf("drain membership/watermark: %+v %+v", role, m.steer)
	}
	payload := m.conv.scrollback.SnapshotAt(m.conv.scrollback.Len() - 1).Payload.(scrollback.UserCardSnapshot)
	if payload.Text != echo.Text || len(payload.Media) != 1 || payload.Media[0] != "image/png (inline)" {
		t.Fatalf("authoritative prompt payload changed: %+v", payload)
	}
	for _, expand := range []bool{false, true} {
		frame := m.rend.renderConversationFrame(&m.conv.scrollback, expand)
		got := ansi.Strip(strings.Join(frame.lines, "\n"))
		want := " ▌ delivered words\n   continuation"
		column := 3
		if expand {
			want = "├▌│ delivered words\n│ │ continuation"
			column = 4
		}
		if !strings.Contains(got, want) || !strings.Contains(got, "📎 image/png (inline)") || strings.Contains(got, "steer") {
			t.Fatalf("delivered ordinary prompt (expanded=%t): %q", expand, got)
		}
		row := frame.firstRegionRow(uint64(id), conversationRegionBody)
		if point, ok := selectionPointFor(frame, row, column); !ok || point.sourceOffset != 0 || !strings.HasPrefix(point.after, "delivered words") {
			t.Fatalf("prompt provenance: %+v %v", point, ok)
		}
	}
	n := m.conv.scrollback.Len()
	m = applyAll(m, echo, client.SteerEchoMsg{MessageID: "steer-0002", Text: "still pending"})
	if m.conv.scrollback.Len() != n+1 || m.steer != nil || m.renderSteer() != "" {
		t.Fatalf("duplicate/tail drain: cards=%d steer=%+v", m.conv.scrollback.Len(), m.steer)
	}
	m.steer = &steerState{Sends: []steerQueuedSend{{ID: "media-only"}}}
	m = applyAll(m, client.SteerEchoMsg{MessageID: "media-only", Parts: echo.Parts})
	if got := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n")); !strings.Contains(got, "├▌│ 📎 image/png (inline)") {
		t.Fatalf("media-only delivered prompt: %q", got)
	}
}

func TestSelectedOutlineWrappedActivityAndDelegation(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m.expandConversation = true
	m.rend.setWidth(32)
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1},
		client.ToolCallMsg{ID: "shell", Name: "Shell", Args: `{"command":"echo one two three four five six seven eight nine ten"}`})
	frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	id := uint64(m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID)
	beforeRenders := m.rend.blockRenders
	if got := ansi.Strip(strings.Join(frame.lines, "\n")); !strings.Contains(got, "├●╮ … running · Shell") {
		t.Fatalf("pending singleton: %q", got)
	}
	argRow := frame.firstRegionRow(id, conversationRegionArguments)
	anchor, ok := frame.anchorForRow(argRow)
	if !ok || !anchor.text {
		t.Fatalf("wrapped argument anchor: %+v", anchor)
	}
	m = applyAll(m, client.ToolCallMsg{ID: "sub", Name: "Subagent", Args: `{"goal":"check the stored facts carefully"}`}, client.SubagentMsg{Kind: client.SubagentStart, ParentCallID: "sub", ChildID: "child", Goal: "check the stored facts carefully"})
	grown := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	got := ansi.Strip(strings.Join(grown.lines, "\n"))
	if strings.Count(got, "●") != 1 || !strings.Contains(got, "├●┬ … running · Shell") || !strings.Contains(got, "│ ╰") || !strings.Contains(got, "Subagent") || strings.Contains(got, "parallel") {
		t.Fatalf("grown batch/delegation: %q", got)
	}
	if m.rend.blockRenders != beforeRenders+2 {
		t.Fatalf("batch growth rerendered old cards: %d -> %d", beforeRenders, m.rend.blockRenders)
	}
	if row, ok := grown.rowForAnchor(anchor); !ok || grown.provenance[row].sourceOffset != anchor.sourceOffset {
		t.Fatal("batch growth lost argument anchor")
	}
	for i, row := range grown.provenance {
		if row.blockID == id && row.region == conversationRegionArguments && !strings.HasPrefix(ansi.Strip(grown.lines[i]), "│ │ ") {
			t.Fatalf("wrapped argument rail: %q", grown.lines[i])
		}
		if ansi.StringWidth(grown.lines[i]) > 32 {
			t.Fatalf("wrapped activity overflow: %q", grown.lines[i])
		}
	}
	m = applyAll(m, client.ToolResultMsg{CallID: "shell", Content: "failed", IsError: true}, client.SubagentMsg{Kind: client.SubagentEnd, ParentCallID: "sub", ChildID: "child", Stop: "error"}, client.ToolResultMsg{CallID: "sub", Content: "child failed", IsError: true})
	got = ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.Contains(got, "├●┬ ✗ Shell") || !strings.Contains(got, "│ ╰ ✗ Subagent") || strings.Count(got, "●") != 1 {
		t.Fatalf("batch error outcomes: %q", got)
	}
}

func TestSelectedOutlineHistoricalUserTailStaysFlat(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(80)
	m.phase = phaseRunning
	m.conv = conversationFromTranscript([]client.ConversationMessage{{Role: "user", Text: "historical prompt"}})
	before := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	id := m.conv.scrollback.MetadataAt(0).ID
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "live content"})
	after := m.rend.renderConversationFrame(&m.conv.scrollback, true)
	if _, assigned := m.conv.scrollback.Outline().Card(id); assigned {
		t.Fatal("snapshot prompt was inferred as a live initial prompt")
	}
	for i, line := range before.lines {
		if before.provenance[i].blockID == uint64(id) && after.lines[i] != line {
			t.Fatalf("snapshot prompt changed: %q -> %q", line, after.lines[i])
		}
	}
}

func TestSelectedOutlinePromptParagraphReflowPreservesSelection(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		for _, gap := range []string{"\n", "\n\n"} {
			t.Run(fmt.Sprintf("delivered=%t/blank=%t", delivered, gap == "\n\n"), func(t *testing.T) {
				m := outlineTestModel()
				m = applyAll(m, tea.WindowSizeMsg{Width: 80, Height: 20})
				m.phase = phaseRunning
				text := "prefixword prefixword prefixword" + gap + "ANCHOR"
				var id scrollback.BlockID
				if delivered {
					m.conv.addUser("initial")
					m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "answer"}, client.TurnEndMsg{Turn: 1})
					m.steer = &steerState{Sends: []steerQueuedSend{{ID: "echo"}}}
					m = applyAll(m, client.SteerEchoMsg{MessageID: "echo", Text: text, Parts: []client.ContentBlock{{Kind: client.ContentBlockImage, MimeType: "image/png", Data: []byte("pixels")}}})
					id = m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID
				} else {
					id = m.conv.addUserWithMedia(text, []string{"image/png (inline)"})
					m = applyAll(m, client.SessionInitMsg{})
				}
				for range 15 {
					m.conv.addNotice("later activity")
				}
				m.refreshView()
				frame := m.conversationView.frame
				row := -1
				for i, p := range frame.provenance {
					if p.blockID == uint64(id) && strings.Contains(ansi.Strip(frame.lines[i]), "ANCHOR") {
						row = i
						break
					}
				}
				if row < 0 {
					t.Fatal("prompt anchor missing")
				}
				m.vp.SetHeight(4)
				m.vp.SetYOffset(row)
				m.conversationView.mode = anchored
				m.conversationView.observe(m.vp)
				m = m.wordSelect(row, 3)
				if !m.sel.active || m.sel.copied != "ANCHOR" || m.sel.anchorPoint.sourceOffset != 32 || m.conversationView.anchor.sourceOffset != 32 {
					t.Fatalf("initial prompt selection/anchor: %+v %+v", m.sel, m.conversationView.anchor)
				}
				for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 16, Height: 20}, tea.KeyPressMsg{Code: tea.KeyF9}, tea.WindowSizeMsg{Width: 80, Height: 20}, tea.KeyPressMsg{Code: tea.KeyF9}} {
					m = applyAll(m, msg)
					if !m.sel.active || m.sel.copied != "ANCHOR" || selectedText(m.vp.GetContent(), m.sel) != "ANCHOR" || m.sel.anchorPoint.sourceOffset != 32 {
						t.Fatalf("selection after %T: %+v; rows=%q", msg, m.sel, m.conversationView.frame.lines)
					}
					if m.conversationView.mode != anchored || m.conversationView.anchor.blockID != uint64(id) || m.conversationView.anchor.sourceOffset != 32 || !strings.Contains(ansi.Strip(m.conversationView.frame.lines[m.vp.YOffset()]), "ANCHOR") {
						t.Fatalf("reading anchor after %T: %+v", msg, m.conversationView.anchor)
					}
					for _, s := range m.conv.testBlocks() {
						if s.ID == id {
							p := s.Payload.(scrollback.UserCardSnapshot)
							if p.Text != text || len(p.Media) != 1 || p.Media[0] != "image/png (inline)" {
								t.Fatalf("prompt payload changed: %+v", p)
							}
						}
					}
					promptRows := []string{}
					for i, p := range m.conversationView.frame.provenance {
						if p.blockID == uint64(id) {
							plain := ansi.Strip(m.conversationView.frame.lines[i])
							promptRows = append(promptRows, graphemeSlice(plain, p.leading, graphemeCount(plain)))
						}
					}
					if got := strings.Join(promptRows, "\n"); !strings.Contains(got, gap+"ANCHOR\n📎 ") {
						t.Fatalf("prompt newline/media layout changed: %q", got)
					}
				}
			})
		}
	}
}

func TestSelectedOutlinePromptRailStyleAndWidth(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	text := "abcdefghijklmn\n\ncontinuation 界 é\x1b[2J"
	first := m.conv.addUserWithMedia(text, []string{"image/png (inline)"})
	m = applyAll(m, client.SessionInitMsg{})
	// Observed initial prompts have their own rail even before any model content.
	m.rend.setWidth(80)
	pre := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	if !strings.HasPrefix(pre, " ▌│ abcdefghijklmn\n  │ \n  │ continuation 界 é") || strings.ContainsAny(pre, "◆╭") {
		t.Fatalf("pre-content initial prompt: %q", pre)
	}
	m = applyAll(m, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "answer\ncontinuation"}, client.TurnEndMsg{Turn: 1})
	m.steer = &steerState{Sends: []steerQueuedSend{{ID: "echo"}}}
	m = applyAll(m, client.SteerEchoMsg{MessageID: "echo", Text: text, Parts: []client.ContentBlock{{Kind: client.ContentBlockImage, MimeType: "image/png", Data: []byte("pixels")}}})
	last := m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID
	userRail := m.rend.th.Style("userLabel").Render("│")
	if userRail == m.rend.th.Style("turnRail").Render("│") || userRail == m.rend.th.Style("runRail").Render("│") {
		t.Fatal("prompt rail must have its independent user semantic style")
	}
	for _, width := range []int{80, 16, 12, 8, 5, 2, 1, 80} {
		m.rend.setWidth(width)
		compact := strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n")
		frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
		for _, id := range []scrollback.BlockID{first, last} {
			count := 0
			var rows []string
			for i, p := range frame.provenance {
				if p.blockID != uint64(id) {
					continue
				}
				line := frame.lines[i]
				rows = append(rows, ansi.Strip(line))
				if ansi.StringWidth(line) > width || strings.Contains(line, "\x1b[2J") {
					t.Fatalf("width=%d unsafe/overflow prompt: %q", width, line)
				}
				if width > 5 {
					outer := " "
					if id == last {
						outer = m.rend.th.Style("runRail").Render("│")
						if count == 0 {
							outer = m.rend.th.Style("runRail").Render("├")
						}
					}
					actor := " "
					if count == 0 {
						actor = m.rend.th.Style("userLabel").Render("▌")
					}
					prefix := outer + actor + userRail + " "
					if !strings.HasPrefix(line, prefix) || p.indent != 4 || p.rail != 4 || p.leading != 4 {
						t.Fatalf("width=%d prompt style/columns: %q %+v", width, line, p)
					}
					if width == 80 {
						body := []string{"abcdefghijklmn", "", "continuation 界 é[2J", m.rend.th.Style("muted").Render("📎 image/png (inline)")}[count]
						if line != prefix+body {
							t.Fatalf("prompt rail recolored body/media: %q", line)
						}
					}
				}
				count++
			}
			if width == 16 && !strings.HasSuffix(rows[0], "abcdefghijkl") {
				t.Fatalf("extra rail was not budgeted before wrapping: %q", rows)
			}
			if width == 80 {
				want := " ▌│ abcdefghijklmn\n  │ \n  │ continuation 界 é[2J\n  │ 📎 image/png (inline)"
				if id == last {
					want = "├▌│ abcdefghijklmn\n│ │ \n│ │ continuation 界 é[2J\n│ │ 📎 image/png (inline)"
				}
				if strings.Join(rows, "\n") != want {
					t.Fatalf("blank/media/body styling: %q", rows)
				}
			}
		}
		loads, renders := m.rend.snapshotLoads, m.rend.blockRenders
		if cached := m.rend.renderConversationFrame(&m.conv.scrollback, true); strings.Join(cached.lines, "\n") != strings.Join(frame.lines, "\n") || m.rend.snapshotLoads != loads || m.rend.blockRenders != renders {
			t.Fatal("unchanged prompt cache was not reused")
		}
		mono := newRenderer(theme.New("monochrome", theme.Palette{}), m.rend.marks)
		mono.setWidth(width)
		if got := strings.Join(mono.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"); ansi.Strip(got) != ansi.Strip(strings.Join(frame.lines, "\n")) {
			t.Fatalf("width=%d monochrome changed prompt geometry", width)
		}
		if got := strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n"); got != compact {
			t.Fatalf("width=%d F9 changed compact prompt bytes", width)
		}
	}
}

func TestSelectedOutlinePromptWrapAndSelection(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m.rend.setWidth(60)
	first := m.conv.addUser("Alpha βeta\nfollowed by enough words to wrap twice")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "answer"}, client.TurnEndMsg{Turn: 1})
	m.steer = &steerState{Sends: []steerQueuedSend{{ID: "echo"}}}
	m = applyAll(m, client.SteerEchoMsg{MessageID: "echo", Text: "Alpha βeta\nfollowed by enough words to wrap twice"})
	last := m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID
	for _, id := range []scrollback.BlockID{first, last} {
		frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
		row := frame.firstRegionRow(uint64(id), conversationRegionBody)
		sel := selection{active: true, anchorL: row, headL: row, anchorC: 10, headC: 14}
		if !sel.snapshotLogical(frame, strings.Join(frame.lines, "\n")) || sel.copied != "βeta" {
			t.Fatalf("prompt selection: %+v", sel)
		}
		anchor, _ := frame.anchorForRow(row)
		for _, width := range []int{24, 16, 60} {
			m.rend.setWidth(width)
			for _, expand := range []bool{false, true} {
				frame = m.rend.renderConversationFrame(&m.conv.scrollback, expand)
				if !sel.resolveLogical(frame) || sel.copied != "βeta" {
					t.Fatalf("prompt reflow selection width %d: %+v", width, sel)
				}
				if _, ok := frame.rowForAnchor(anchor); !ok {
					t.Fatal("prompt reflow lost anchor")
				}
				for i, p := range frame.provenance {
					if p.blockID != uint64(id) {
						continue
					}
					plain := ansi.Strip(frame.lines[i])
					if ansi.StringWidth(plain) > width {
						t.Fatalf("prompt width %d: %q", width, plain)
					}
					if p.sourceOffset > 0 {
						want := "   "
						if expand {
							want = "  │ "
							if id == last {
								want = "│ │ "
							}
						}
						if !strings.HasPrefix(plain, want) {
							t.Fatalf("prompt continuation alignment: %q", plain)
						}
					}
				}
			}
		}
	}
}
