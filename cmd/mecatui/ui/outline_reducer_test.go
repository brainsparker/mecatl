package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stacklok/mecatl/cmd/mecatui/client"
	"github.com/stacklok/mecatl/cmd/mecatui/theme"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/scrollback"
)

func outlineTestModel() Model {
	return newTestModelFromDeps(Deps{Theme: theme.New("aztec", theme.AztecPalette())})
}

func TestOutlineEmptyInterruptedRunsStayChronological(t *testing.T) {
	for _, terminal := range []tea.Msg{client.ResultMsg{Stop: "cancelled"}, client.StreamClosedMsg{}} {
		t.Run(fmt.Sprintf("%T", terminal), func(t *testing.T) {
			m := outlineTestModel()
			m.rend.setWidth(80)
			for _, prompt := range []string{"old prompt", "middle prompt"} {
				m.conv.addUser(prompt)
				m.phase = phaseRunning
				m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, terminal)
			}
			m.conv.addUser("new prompt")
			m.phase = phaseRunning
			m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "new answer"}, client.TurnEndMsg{Turn: 1}, client.ResultMsg{Stop: "end_turn"})
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			plain := stripANSIstr(strings.Join(frame.lines, "\n"))
			old, middle, newer := strings.Index(plain, "old prompt"), strings.Index(plain, "middle prompt"), strings.Index(plain, "new prompt")
			if old < 0 || middle <= old || newer <= middle || strings.Count(plain[old:middle], "╰─ interrupted") != 1 || strings.Count(plain[middle:newer], "╰─ interrupted") != 1 || strings.Contains(plain[newer:], "interrupted") {
				t.Fatalf("empty closures crossed prompts or duplicated:\n%s", plain)
			}
			if strings.Count(plain, "no visible output") != 2 || strings.Count(plain, "◆") != 3 {
				t.Fatalf("missing textless-call annotation:\n%s", plain)
			}
		})
	}
}

func TestOutlineCancellationResultAndUsage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		events    []tea.Msg
		result    client.ResultMsg
		status    scrollback.RunStatus
		available bool
		footer    string
		diamonds  int
	}{
		{name: "before call", result: client.ResultMsg{Stop: "cancelled"}, status: scrollback.RunInterrupted, footer: "╰─ interrupted"},
		{name: "before output", events: []tea.Msg{client.TurnStartMsg{Turn: 1}}, result: client.ResultMsg{Stop: "cancelled"}, status: scrollback.RunInterrupted, footer: "╰─ interrupted", diamonds: 1},
		{name: "after output unknown", events: []tea.Msg{client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "partial answer"}}, result: client.ResultMsg{Stop: "cancelled"}, status: scrollback.RunInterrupted, footer: "╰─ interrupted", diamonds: 1},
		{name: "unfinished measured", events: []tea.Msg{client.TurnStartMsg{Turn: 1}}, result: client.ResultMsg{Stop: "cancelled", Usage: client.Usage{InputTokens: 12, OutputTokens: 3}}, status: scrollback.RunInterrupted, available: true, footer: "╰─ ↑12 ↓3 · interrupted", diamonds: 1},
		{name: "completed known zero", events: []tea.Msg{client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1}}, result: client.ResultMsg{Stop: "cancelled"}, status: scrollback.RunInterrupted, available: true, footer: "╰─ ↑0 ↓0 · interrupted", diamonds: 1},
		{name: "after tools and hidden call", events: []tea.Msg{client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1, Usage: client.Usage{InputTokens: 12, OutputTokens: 3}, DurationMs: 500}, client.ToolCallMsg{ID: "read", Name: "Read", Args: `{}`}, client.ToolResultMsg{CallID: "read", Content: "ok"}, client.TurnStartMsg{Turn: 2}}, result: client.ResultMsg{Stop: "cancelled", Usage: client.Usage{InputTokens: 12, OutputTokens: 3}}, status: scrollback.RunInterrupted, available: true, footer: "╰─ ↑12 ↓3 · LLM 500ms · tools 1s · interrupted", diamonds: 2},
		{name: "normal wins cancel request", events: []tea.Msg{client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "finished answer"}, client.TurnEndMsg{Turn: 1}}, result: client.ResultMsg{Stop: "end_turn"}, status: scrollback.RunNormal, available: true, footer: "╰─ ↑0 ↓0", diamonds: 1},
		{name: "error wins cancel request", events: []tea.Msg{client.TurnStartMsg{Turn: 1}}, result: client.ResultMsg{Stop: "error", Error: "failed", Usage: client.Usage{InputTokens: 7}}, status: scrollback.RunError, available: true, footer: "╰─ ↑7 ↓0 · error", diamonds: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := outlineTestModel()
			m.rend.setWidth(90)
			m.conv.addUser("old prompt")
			m.phase = phaseRunning
			m = applyAll(m, client.SessionInitMsg{})
			m = applyAll(m, tc.events...)
			m = applyAll(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			run, _ := m.conv.scrollback.Outline().Run(m.outlineRun)
			if run.Status != scrollback.RunOpen || !strings.Contains(m.statusMsg, "cancelling") {
				t.Fatalf("local cancel classified run or did not request cancellation: %+v, %q", run, m.statusMsg)
			}
			m = applyAll(m, tc.result)
			run = m.conv.scrollback.Outline().Runs()[0]
			if run.Status != tc.status || run.UsageAvailable != tc.available || run.Usage != outlineUsage(tc.result.Usage) {
				t.Fatalf("result projection: %+v", run)
			}
			m.conv.addUser("next prompt")
			plain := stripANSIstr(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
			footer, next := strings.Index(plain, tc.footer), strings.Index(plain, "next prompt")
			if footer < 0 || next <= footer || strings.Count(plain, "\n╰─ ") != 1 || strings.Count(plain, "◆") != tc.diamonds || (!tc.available && strings.Contains(plain, "↑0 ↓0")) || (tc.status != scrollback.RunInterrupted && strings.Contains(plain, "interrupted")) {
				t.Fatalf("terminal chrome/status/usage:\n%s", plain)
			}
		})
	}
}

func TestLiveOutlineLifecycle(t *testing.T) {
	m := outlineTestModel()
	m = applyAll(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	prompt := m.conv.addUser("inspect")
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{})
	if role, ok := m.conv.scrollback.Outline().Card(prompt); !ok || !role.InitialPrompt || role.RunID != m.outlineRun {
		t.Fatal("initial prompt lost observed run association")
	}
	if m.outlineRun == 0 || len(m.conv.scrollback.Outline().Runs()) != 1 {
		t.Fatal("session.init did not open one observed run")
	}
	m = applyAll(m, client.TurnStartMsg{Turn: 1})
	first := m.conv.scrollback.Outline().Runs()[0]
	if len(first.Turns) != 1 || first.Turns[0].CardID == 0 || first.Turns[0].Visible || first.Turns[0].Ended {
		t.Fatalf("open first turn = %+v", first.Turns)
	}
	m = applyAll(m,
		client.ReasoningDeltaMsg{Turn: 1, Text: "thinking"},
		client.TurnEndMsg{Turn: 1, Usage: client.Usage{InputTokens: 5, OutputTokens: 2}, DurationMs: 300, Estimated: true},
		client.ToolCallMsg{ID: "read", Name: "Read", Args: `{}`},
		client.ToolCallMsg{ID: "grep", Name: "Grep", Args: `{}`},
	)
	run := m.conv.scrollback.Outline().Runs()[0]
	if !run.Turns[0].Visible || !run.Turns[0].Ended || run.Turns[0].Stats != (scrollback.TurnStats{Usage: scrollback.Usage{InputTokens: 5, OutputTokens: 2}, DurationMs: 300, Estimated: true}) {
		t.Fatalf("reasoning turn stats = %+v", run.Turns[0])
	}
	if len(run.Tools) != 2 || run.ToolDurationMs != 1500 {
		t.Fatalf("tool batch = %+v", run)
	}
	stat := m.conv.scrollback.MetadataAt(2)
	if stat.Kind != scrollback.KindTurnStat {
		t.Fatalf("expected turn-stat card, got %+v", stat)
	}
	statOutline, ok := m.conv.scrollback.Outline().Card(stat.ID)
	if !ok || statOutline.RunID != run.ID || statOutline.TurnID != run.Turns[0].ID {
		t.Fatalf("turn-stat membership = %+v, %v", statOutline, ok)
	}
	for _, tool := range run.Tools {
		card, ok := m.conv.scrollback.Outline().Card(tool.CardID)
		if !ok || !card.Tool || card.TurnID != 0 || card.RunID != run.ID {
			t.Fatalf("tool card membership = %+v, %v", card, ok)
		}
	}
	m = applyAll(m, client.TurnStartMsg{Turn: 2}, client.TurnEndMsg{Turn: 2})
	run = m.conv.scrollback.Outline().Runs()[0]
	if len(run.Turns) != 2 || run.Turns[1].Visible || !run.Turns[1].Ended {
		t.Fatalf("textless second turn = %+v", run.Turns)
	}
	m = applyAll(m, client.ToolCallMsg{ID: "write", Name: "Write", Args: `{}`})
	m = applyAll(m, client.ResultMsg{Stop: "end_turn", Usage: client.Usage{InputTokens: 100, OutputTokens: 40}})
	run = m.conv.scrollback.Outline().Runs()[0]
	if run.Status != scrollback.RunNormal || !run.UsageAvailable || run.Usage.InputTokens != 100 || run.Usage.OutputTokens != 40 || run.LLMDurationMs != 300 || run.ToolDurationMs != 3500 {
		t.Fatalf("terminal run = %+v", run)
	}
	if m.usage.InputTokens != 100 || m.usage.OutputTokens != 40 {
		t.Fatalf("session usage double-counted turn stats: %+v", m.usage)
	}
	if m.outlineRun != 0 || m.outlineTurn != 0 {
		t.Fatalf("terminal IDs retained: %d %d", m.outlineRun, m.outlineTurn)
	}
	if got := stripANSIstr(m.rend.renderConversation(&m.conv, false)); strings.Contains(got, "↑5 ↓2") {
		t.Fatalf("collapsed view exposed trivial stat: %q", got)
	}
	if got := stripANSIstr(m.rend.renderConversation(&m.conv, true)); !strings.Contains(got, "↑5 ↓2") {
		t.Fatalf("expanded view lost trivial stat: %q", got)
	}
}

func TestLiveOutlineInterruptedAndReconnect(t *testing.T) {
	m := outlineTestModel()
	m.rend.setWidth(72)
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
	id := m.outlineRun
	m = applyAll(m, reconnectMsg{gen: m.liveReconGen, msg: client.LiveReconnectingMsg{Attempt: 1}})
	run, _ := m.conv.scrollback.Outline().Run(id)
	if run.Status != scrollback.RunOpen || !run.Reconnecting {
		t.Fatalf("reconnecting run = %+v", run)
	}
	if frame := stripANSIstr(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n")); strings.Contains(frame, "╭") || strings.Contains(frame, "╰─") {
		t.Fatalf("reconnect closed open turn rail: %q", frame)
	}
	m = applyAll(m, reconnectMsg{gen: m.liveReconGen, msg: client.LiveReconnectedMsg{}})
	run, _ = m.conv.scrollback.Outline().Run(id)
	if run.Status != scrollback.RunOpen || run.Reconnecting {
		t.Fatalf("reconnected run = %+v", run)
	}
	m = applyAll(m, client.StreamErrMsg{Err: errors.New("stream lost")})
	run, _ = m.conv.scrollback.Outline().Run(id)
	if run.Status != scrollback.RunInterrupted || run.UsageAvailable {
		t.Fatalf("interrupted run = %+v", run)
	}
	m = applyAll(m, client.SessionInitMsg{}, client.ResultMsg{Stop: "error", Usage: client.Usage{InputTokens: 9}})
	run = m.conv.scrollback.Outline().Runs()[1]
	if run.Status != scrollback.RunError || run.Usage.InputTokens != 9 {
		t.Fatalf("error run = %+v", run)
	}
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{})
	third := m.outlineRun
	m = applyAll(m, streamMsg{gen: m.streamGen - 1, msg: client.StreamErrMsg{Err: errors.New("stale reader")}})
	if run, _ := m.conv.scrollback.Outline().Run(third); run.Status != scrollback.RunOpen {
		t.Fatalf("stale stream error closed new run: %+v", run)
	}
	m = applyAll(m, client.StreamClosedMsg{})
	if got := m.conv.scrollback.Outline().Runs()[2].Status; got != scrollback.RunInterrupted {
		t.Fatalf("stream close status = %v", got)
	}
	m = m.resetSession()
	if m.outlineRun != 0 || m.outlineTurn != 0 || len(m.conv.scrollback.Outline().Runs()) != 0 {
		t.Fatal("session reset retained outline")
	}
}

func TestDuplicateSessionInitKeepsOpenOutline(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
	id, turn := m.outlineRun, m.outlineTurn
	m = applyAll(m, client.SessionInitMsg{}, client.AssistantDeltaMsg{Turn: 1, Text: "still active"})
	if m.outlineRun != id || m.outlineTurn != turn || len(m.conv.scrollback.Outline().Runs()) != 1 {
		t.Fatalf("duplicate init replaced live outline: run=%d turn=%d", m.outlineRun, m.outlineTurn)
	}
}

func TestSnapshotTranscriptRemainsFlat(t *testing.T) {
	loaded := conversationFromTranscript([]client.ConversationMessage{
		{Role: "user", Text: "historical prompt"},
		{Role: "assistant", Text: "historical reply"},
	})
	m := outlineTestModel()
	m.conv = loaded
	if got := m.conv.scrollback.Outline().Runs(); len(got) != 0 {
		t.Fatalf("snapshot inferred run boundaries: %+v", got)
	}
	for i := 0; i < m.conv.scrollback.Len(); i++ {
		if _, ok := m.conv.scrollback.Outline().Card(m.conv.scrollback.MetadataAt(i).ID); ok {
			t.Fatalf("snapshot card %d acquired outline membership", i)
		}
	}
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
	if len(m.conv.scrollback.Outline().Runs()) != 1 {
		t.Fatal("new live run not observed after flat snapshot")
	}
	for i := 0; i < 2; i++ {
		if _, ok := m.conv.scrollback.Outline().Card(m.conv.scrollback.MetadataAt(i).ID); ok {
			t.Fatalf("historical card %d absorbed by new run", i)
		}
	}
}

func TestQueuedRunsKeepPromptsOutsideRails(t *testing.T) {
	m, _ := newQueueModel(t)
	m = startRunning(t, m, "first")
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1})
	m = enqueue(t, m, "second")
	m = applyAll(m, client.ResultMsg{Stop: "end_turn"})
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1})
	runs := m.conv.scrollback.Outline().Runs()
	if len(runs) != 2 || runs[0].Status != scrollback.RunNormal || runs[1].Status != scrollback.RunOpen || runs[0].ID == runs[1].ID {
		t.Fatalf("queued run boundaries = %+v", runs)
	}
	for i := 0; i < m.conv.scrollback.Len(); i++ {
		card := m.conv.scrollback.MetadataAt(i)
		if card.Kind == scrollback.KindUser {
			if role, ok := m.conv.scrollback.Outline().Card(card.ID); !ok || !role.InitialPrompt {
				t.Fatalf("user prompt %d lost initial-prompt role", card.ID)
			}
		}
	}
	frame := stripANSIstr(strings.Join(m.rend.renderConversationFrame(&m.conv.scrollback, true).lines, "\n"))
	firstFooter, secondPrompt := strings.Index(frame, "╰─"), strings.Index(frame, "second")
	if firstFooter < 0 || secondPrompt <= firstFooter || strings.Contains(frame[secondPrompt:], "╭") {
		t.Fatalf("queued run rendering crossed prompt boundary: %q", frame)
	}
}
