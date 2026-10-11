package scrollback

import "testing"

func TestOutlineCancelledUsageAvailability(t *testing.T) {
	for _, tc := range []struct {
		name      string
		usage     Usage
		ended     bool
		available bool
	}{
		{name: "unfinished missing"},
		{name: "completed zero", ended: true, available: true},
		{name: "input", usage: Usage{InputTokens: 5}, available: true},
		{name: "output", usage: Usage{OutputTokens: 5}, available: true},
		{name: "cache read", usage: Usage{CacheReadTokens: 5}, available: true},
		{name: "cache write", usage: Usage{CacheWriteTokens: 5}, available: true},
		{name: "reasoning", usage: Usage{ReasoningTokens: 5}, available: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c Conversation
			o := c.Outline()
			run := o.StartRun(0)
			turn := o.StartTurn(run, c.Messages().AddAssistant(AssistantInput{}))
			if tc.ended && !o.EndTurn(turn, TurnStats{}) {
				t.Fatal("end turn")
			}
			if !o.EndRun(run, RunInterrupted, tc.usage) {
				t.Fatal("record cancelled terminal")
			}
			got, _ := o.Run(run)
			if got.Status != RunInterrupted || got.Usage != tc.usage || got.UsageAvailable != tc.available {
				t.Fatalf("cancelled usage: %+v", got)
			}
		})
	}
}

func TestOutlineTurnsRecordVisibleAndTextlessBoundaries(t *testing.T) {
	var c Conversation
	outline := c.Outline()
	run := outline.StartRun(0)
	firstCard := c.Messages().AddAssistant(AssistantInput{})
	first := outline.StartTurn(run, firstCard)
	if first == 0 || !outline.MarkTurnVisible(first) || !outline.EndTurn(first, TurnStats{Usage: Usage{InputTokens: 10, OutputTokens: 2}, DurationMs: 500}) {
		t.Fatal("complete visible first turn")
	}
	secondCard := c.Messages().AddAssistant(AssistantInput{})
	second := outline.StartTurn(run, secondCard)
	if second == 0 || !outline.EndTurn(second, TurnStats{}) {
		t.Fatal("complete textless later turn")
	}

	snapshot, ok := outline.Run(run)
	if !ok || len(snapshot.Turns) != 2 {
		t.Fatalf("Run(%d) = %#v, %t", run, snapshot, ok)
	}
	if got := snapshot.Turns[0]; !got.Visible || !got.Ended || got.CardID != firstCard || got.Stats.DurationMs != 500 {
		t.Fatalf("first turn = %#v", got)
	}
	if got := snapshot.Turns[1]; got.Visible || !got.Ended || got.CardID != secondCard {
		t.Fatalf("second turn = %#v", got)
	}
	if card, ok := outline.Card(firstCard); !ok || card.RunID != run || card.TurnID != first || card.Tool {
		t.Fatalf("first card outline = %#v, %t", card, ok)
	}
}

func TestOutlineTurnsRecordTextlessFirstAndVisibleLater(t *testing.T) {
	var c Conversation
	o := c.Outline()
	run := o.StartRun(0)
	firstCard := c.Messages().AddAssistant(AssistantInput{})
	first := o.StartTurn(run, firstCard)
	if first == 0 || !o.EndTurn(first, TurnStats{}) {
		t.Fatal("complete textless first turn")
	}
	secondCard := c.Messages().AddAssistant(AssistantInput{})
	second := o.StartTurn(run, secondCard)
	if second == 0 || !o.MarkTurnVisible(second) || !o.EndTurn(second, TurnStats{}) {
		t.Fatal("complete visible later turn")
	}
	snapshot, _ := o.Run(run)
	if got := snapshot.Turns[0]; got.Visible || !got.Ended || got.CardID != firstCard {
		t.Fatalf("textless first turn = %#v", got)
	}
	if got := snapshot.Turns[1]; !got.Visible || !got.Ended || got.CardID != secondCard {
		t.Fatalf("visible later turn = %#v", got)
	}
}

func TestOutlineToolDurationsAreStableAndBatchTotalsUseMaxThenSum(t *testing.T) {
	var c Conversation
	outline := c.Outline()
	run := outline.StartRun(0)
	turnCard := c.Messages().AddAssistant(AssistantInput{})
	turn := outline.StartTurn(run, turnCard)
	if !outline.EndTurn(turn, TurnStats{DurationMs: 1200}) {
		t.Fatal("end first turn")
	}
	firstTool := c.Tools().Add(ToolCall{ID: "one"})
	secondTool := c.Tools().Add(ToolCall{ID: "two"})
	firstDuration, ok := outline.AddTool(run, firstTool)
	if !ok {
		t.Fatal("add first tool")
	}
	secondDuration, ok := outline.AddTool(run, secondTool)
	if !ok {
		t.Fatal("add second tool")
	}
	if repeated, ok := outline.AddTool(run, firstTool); !ok || repeated != firstDuration {
		t.Fatalf("repeated tool duration = %d, %t; want %d, true", repeated, ok, firstDuration)
	}

	secondTurnCard := c.Messages().AddAssistant(AssistantInput{})
	secondTurn := outline.StartTurn(run, secondTurnCard)
	if !outline.EndTurn(secondTurn, TurnStats{DurationMs: -1}) {
		t.Fatal("end second turn")
	}
	thirdTool := c.Tools().Add(ToolCall{ID: "three"})
	thirdDuration, ok := outline.AddTool(run, thirdTool)
	if !ok {
		t.Fatal("add third tool")
	}
	if !outline.EndRun(run, RunNormal, Usage{InputTokens: 50, OutputTokens: 5}) {
		t.Fatal("end run")
	}

	snapshot, _ := outline.Run(run)
	if snapshot.Tools[0].BatchID == 0 || snapshot.Tools[0].BatchID != snapshot.Tools[1].BatchID || snapshot.Tools[1].BatchID == snapshot.Tools[2].BatchID {
		t.Fatalf("stored synthetic batch membership = %+v", snapshot.Tools)
	}
	wantTools := max(firstDuration, secondDuration) + thirdDuration
	if snapshot.ToolDurationMs != wantTools {
		t.Fatalf("tool duration = %d, want max(%d, %d) + %d = %d", snapshot.ToolDurationMs, firstDuration, secondDuration, thirdDuration, wantTools)
	}
	if snapshot.LLMDurationMs != 1200 {
		t.Fatalf("LLM duration = %d, want 1200", snapshot.LLMDurationMs)
	}
	if got, ok := outline.Card(secondTool); !ok || got.DurationMs != secondDuration || !got.Tool {
		t.Fatalf("second tool outline = %#v, %t", got, ok)
	}
	t.Run("tool before any turn end is not batch time", func(t *testing.T) {
		var early Conversation
		o := early.Outline()
		id := o.StartRun(0)
		tool := early.Tools().Add(ToolCall{ID: "early"})
		if duration, ok := o.AddTool(id, tool); !ok || duration <= 0 {
			t.Fatalf("early tool duration = %d, %v", duration, ok)
		}
		if got, ok := o.Run(id); !ok || got.ToolDurationMs != 0 {
			t.Fatalf("unbatched tool contributed to aggregate: %+v", got)
		}
		if !o.EndRun(id, RunNormal, Usage{}) {
			t.Fatal("end run")
		}
		if got, _ := o.Run(id); got.ToolDurationMs != 0 {
			t.Fatalf("unbatched terminal tools = %dms", got.ToolDurationMs)
		}
	})
}

func TestOutlineRunTerminalAndReconnectStates(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		var c Conversation
		o := c.Outline()
		run := o.StartRun(0)
		if !o.EndRun(run, RunNormal, Usage{InputTokens: 1}) {
			t.Fatal("end normal run")
		}
		got, _ := o.Run(run)
		if got.Status != RunNormal || !got.UsageAvailable || got.Usage.InputTokens != 1 {
			t.Fatalf("run = %#v", got)
		}
	})
	t.Run("error", func(t *testing.T) {
		var c Conversation
		o := c.Outline()
		run := o.StartRun(0)
		if !o.EndRun(run, RunError, Usage{}) {
			t.Fatal("end error run")
		}
		got, _ := o.Run(run)
		if got.Status != RunError || !got.UsageAvailable {
			t.Fatalf("run = %#v", got)
		}
	})
	t.Run("reconnecting remains open", func(t *testing.T) {
		var c Conversation
		o := c.Outline()
		run := o.StartRun(0)
		if !o.SetReconnecting(run, true) {
			t.Fatal("set reconnecting")
		}
		got, _ := o.Run(run)
		if got.Status != RunOpen || !got.Reconnecting {
			t.Fatalf("run = %#v", got)
		}
		if !o.SetReconnecting(run, false) {
			t.Fatal("clear reconnecting")
		}
	})
	t.Run("interrupted", func(t *testing.T) {
		var c Conversation
		o := c.Outline()
		run := o.StartRun(0)
		if !o.Interrupt(run) {
			t.Fatal("interrupt run")
		}
		got, _ := o.Run(run)
		if got.Status != RunInterrupted || got.UsageAvailable {
			t.Fatalf("run = %#v", got)
		}
	})
}

func TestOutlineLeavesUntrackedCardsAloneAndSnapshotsDetached(t *testing.T) {
	var c Conversation
	user := c.Messages().AddUser(UserInput{Text: "outside run"})
	assistant := c.Messages().AddAssistant(AssistantInput{})
	tool := c.Tools().Add(ToolCall{ID: "tool"})
	if before := c.MetadataAt(1); before.Revision != 0 {
		t.Fatalf("assistant revision before outline = %d, want 0", before.Revision)
	}
	o := c.Outline()
	run := o.StartRun(0)
	turn := o.StartTurn(run, assistant)
	if !o.MarkTurnVisible(turn) || !o.EndTurn(turn, TurnStats{Usage: Usage{InputTokens: 3}}) {
		t.Fatal("complete turn")
	}
	if _, ok := o.AddTool(run, tool); !ok {
		t.Fatal("add tool")
	}
	if _, ok := o.Card(user); ok {
		t.Fatal("untracked user acquired outline metadata")
	}
	if got := c.MetadataAt(1).Revision; got != 0 {
		t.Fatalf("assistant revision after outline = %d, want 0", got)
	}

	snapshot, _ := o.Run(run)
	snapshot.Turns[0].Visible = false
	snapshot.Turns[0].Stats.InputTokens = 99
	snapshot.Tools[0].DurationMs = 99
	again, _ := o.Run(run)
	if !again.Turns[0].Visible || again.Turns[0].Stats.InputTokens != 3 || again.Tools[0].DurationMs == 99 {
		t.Fatalf("snapshot mutation leaked: %#v", again)
	}
	runs := o.Runs()
	runs[0].Turns = nil
	again, _ = o.Run(run)
	if len(again.Turns) != 1 {
		t.Fatalf("Runs snapshot mutation leaked: %#v", again)
	}
}
