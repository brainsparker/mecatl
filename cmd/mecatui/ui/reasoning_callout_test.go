package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stacklok/mecatl/cmd/mecatui/client"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/scrollback"
)

func TestCompactReasoningCallout(t *testing.T) {
	for _, answer := range []string{"", "visible answer"} {
		for width := 1; width <= 80; width++ {
			t.Run(fmt.Sprintf("answer=%t/width=%d", answer != "", width), func(t *testing.T) {
				m := outlineTestModel()
				m.rend.setWidth(width)
				m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.ReasoningDeltaMsg{Turn: 1, Text: "  PRIVATE first\n\n  \nPRIVATE last\n\n "})
				if answer != "" {
					m = applyAll(m, client.AssistantDeltaMsg{Turn: 1, Text: answer})
				}
				id := uint64(m.conv.scrollback.MetadataAt(0).ID)
				payload := m.conv.scrollback.SnapshotAt(0).Payload.(scrollback.AssistantCardSnapshot)
				m.rend.outlineInset = 4
				callout := ansi.Strip(m.rend.renderReasoningSnapshot(payload, false))
				m.rend.outlineInset = 0
				if got := strings.ReplaceAll(callout, "\n", ""); strings.ReplaceAll(got, " ", "") != "reasoning·~4lines·f9expand" {
					t.Fatalf("source count/header: %q", callout)
				}
				frame := m.rend.renderConversationFrame(&m.conv.scrollback, false)
				if strings.Contains(ansi.Strip(strings.Join(frame.lines, "\n")), "PRIVATE") || frame.hasRegion(id, conversationRegionReasoning) {
					t.Fatalf("collapsed reasoning leaked body/source region: %q", frame.lines)
				}
				headerRows := len(strings.Split(callout, "\n"))
				for i := 0; i < headerRows; i++ {
					p := frame.provenance[i]
					if p.blockID != id || p.region != conversationRegionChrome || p.text {
						t.Fatalf("header row %d not owned chrome: %+v", i, p)
					}
					if _, ok := selectionPointFor(frame, i, p.rail); ok {
						t.Fatalf("header row %d selectable", i)
					}
				}
				if answer == "" && frame.hasRegion(id, conversationRegionBody) {
					t.Fatal("reasoning-only call invented answer provenance")
				}
				for _, expand := range []bool{false, true} {
					frame = m.rend.renderConversationFrame(&m.conv.scrollback, expand)
					for _, line := range frame.lines {
						if ansi.StringWidth(line) > width {
							t.Fatalf("overflow: %q", line)
						}
					}
					if width >= 5 && strings.Count(ansi.Strip(strings.Join(frame.lines, "\n")), "◆") != 1 {
						t.Fatal("call must own exactly one diamond")
					}
					if answer != "" {
						row := frame.firstRegionRow(id, conversationRegionBody)
						point, ok := selectionPointFor(frame, row, frame.provenance[row].rail)
						if !ok || point.sourceOffset != 0 || !strings.HasPrefix(point.after, "v") {
							t.Fatalf("answer offset changed: %+v %v", point, ok)
						}
					}
					if expand {
						body := ansi.Strip(m.rend.renderReasoningSnapshot(payload, true))
						if strings.Contains(body, "f9") || strings.Contains(body, "~4") || !strings.Contains(strings.ReplaceAll(body, "\n", ""), "PRIVATE") || !frame.hasRegion(id, conversationRegionReasoning) {
							t.Fatalf("expanded reasoning: %q", body)
						}
					}
				}
			})
		}
	}
}

func TestCompactReasoningStreamingAndEmpty(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(80)
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.ReasoningDeltaMsg{Turn: 1, Text: "PRIVATE first\n"})
	for _, count := range []int{1, 2, 3} {
		got := ansi.Strip(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, false).lines, "\n"))
		if !strings.Contains(got, " ◆  reasoning · ~"+plural(count, "line")+" · f9 expand") || strings.Contains(got, "PRIVATE") || strings.Count(got, "◆") != 1 {
			t.Fatalf("streaming count %d: %q", count, got)
		}
		m = applyAll(m, client.ReasoningDeltaMsg{Turn: 1, Text: "PRIVATE next\n"})
	}
	for _, expand := range []bool{false, true} {
		for _, reasoning := range []string{"", " \n\t\n ", "\x00\r\n\u200b "} {
			p := scrollback.AssistantCardSnapshot{Text: "answer", Reasoning: reasoning}
			if got := m.rend.renderReasoningSnapshot(p, expand); got != "" {
				t.Fatalf("empty sanitized reasoning: %q", got)
			}
			out := ansi.Strip(m.rend.renderSnapshot(0, testSnapshot(0, p), expand))
			if strings.Contains(out, "reasoning") || strings.Contains(out, "f9") || !strings.Contains(out, "answer") {
				t.Fatalf("whitespace reasoning changed answer/chrome: %q", out)
			}
		}
	}
}

func TestCompactReasoningF9AnswerSelectionAndAnchor(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m = applyAll(m, tea.WindowSizeMsg{Width: 80, Height: 20}, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.ReasoningDeltaMsg{Turn: 1, Text: strings.Repeat("private reasoning\n", 20)}, client.AssistantDeltaMsg{Turn: 1, Text: "prefixwordprefixwordprefixwordxx\n\nANCHOR"})
	for range 15 {
		m.conv.addNotice("later activity")
	}
	m.refreshView()
	row := -1
	for i, line := range m.conversationView.frame.lines {
		if strings.Contains(ansi.Strip(line), "ANCHOR") {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatal("answer missing")
	}
	m.vp.SetHeight(4)
	m.vp.SetYOffset(row)
	m.conversationView.mode = anchored
	m.conversationView.observe(m.vp)
	m = m.wordSelect(row, 4)
	for _, msg := range []tea.Msg{tea.KeyPressMsg{Code: tea.KeyF9}, tea.WindowSizeMsg{Width: 16, Height: 20}, tea.KeyPressMsg{Code: tea.KeyF9}, tea.WindowSizeMsg{Width: 80, Height: 20}} {
		m = applyAll(m, msg)
		if !m.sel.active || m.sel.copied != "ANCHOR" || selectedText(m.vp.GetContent(), m.sel) != "ANCHOR" || m.sel.anchorPoint.sourceOffset != 32 {
			t.Fatalf("selection after %T: %+v", msg, m.sel)
		}
		if m.conversationView.mode != anchored || m.conversationView.anchor.region != conversationRegionBody || m.conversationView.anchor.sourceOffset != 32 || !strings.Contains(ansi.Strip(m.conversationView.frame.lines[m.vp.YOffset()]), "ANCHOR") {
			t.Fatalf("reading anchor after %T: %+v mode=%v offset=%d line=%q", msg, m.conversationView.anchor, m.conversationView.mode, m.vp.YOffset(), ansi.Strip(m.conversationView.frame.lines[m.vp.YOffset()]))
		}
	}
}

func TestCompactReasoningCollapseInvalidatesSelection(t *testing.T) {
	for _, answer := range []string{"", "answer"} {
		m := outlineTestModel()
		m.phase = phaseRunning
		m = applyAll(m, tea.WindowSizeMsg{Width: 80, Height: 20}, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.ReasoningDeltaMsg{Turn: 1, Text: "PRIVATE reasoning"})
		if answer != "" {
			m = applyAll(m, client.AssistantDeltaMsg{Turn: 1, Text: answer})
		}
		for range 10 {
			m.conv.addNotice("later activity")
		}
		m = applyAll(m, tea.KeyPressMsg{Code: tea.KeyF9})
		frame := m.conversationView.frame
		row := frame.firstRegionRow(uint64(m.conv.scrollback.MetadataAt(0).ID), conversationRegionReasoning)
		m.vp.SetHeight(4)
		m.vp.SetYOffset(row)
		m.conversationView.mode = anchored
		m.conversationView.observe(m.vp)
		m = m.wordSelect(row, 4)
		if !m.sel.active || m.sel.copied != "PRIVATE" {
			t.Fatalf("expanded reasoning not selectable: %+v", m.sel)
		}
		m = applyAll(m, tea.KeyPressMsg{Code: tea.KeyF9})
		if m.sel.active || strings.Contains(m.vp.GetContent(), "PRIVATE") {
			t.Fatalf("collapsed reasoning retained selection/body: %+v", m.sel)
		}
		p := m.conversationView.frame.provenance[m.vp.YOffset()]
		wantRegion := conversationRegionChrome
		if answer != "" {
			wantRegion = conversationRegionBody
		}
		if m.conversationView.mode != anchored || p.blockID != 1 || p.region != wantRegion || p.text != (answer != "") {
			t.Fatalf("reasoning anchor collapse left its owning call: %+v", p)
		}
	}
}
