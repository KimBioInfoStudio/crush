package chat

import (
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// TestAssistantMessageItemExpandable guards the Expandable contract on
// AssistantMessageItem. The earlier implementation returned no value, which
// meant the type silently did not satisfy chat.Expandable and the
// keyboard-driven expand path in model/chat.go skipped thinking blocks.
func TestAssistantMessageItemExpandable(t *testing.T) {
	t.Parallel()

	sty := styles.CharmtonePantera()
	msg := &message.Message{ID: "m1", Role: message.Assistant}
	item := NewAssistantMessageItem(&sty, msg)

	exp, ok := item.(Expandable)
	require.True(t, ok, "AssistantMessageItem must satisfy Expandable")

	require.True(t, exp.ToggleExpanded(), "first toggle should report expanded")
	require.False(t, exp.ToggleExpanded(), "second toggle should report collapsed")
}

// TestAssistantMessageItemHandleMouseClick ensures HandleMouseClick does not
// toggle expansion on its own. The generic Expandable path in
// model/chat.go does the toggle; doing it here too would double-toggle and
// net to no change.
func TestAssistantMessageItemHandleMouseClick(t *testing.T) {
	t.Parallel()

	sty := styles.CharmtonePantera()
	msg := &message.Message{ID: "m2", Role: message.Assistant}
	item := NewAssistantMessageItem(&sty, msg).(*AssistantMessageItem)
	item.thinkingBoxHeight = 5

	// Click inside the thinking box signals handled but must not mutate
	// the expanded state.
	require.True(t, item.HandleMouseClick(ansi.MouseLeft, 0, 2))
	require.False(t, item.thinkingExpanded, "HandleMouseClick must not toggle expansion on its own")

	// Click outside the thinking box is ignored entirely.
	require.False(t, item.HandleMouseClick(ansi.MouseLeft, 0, 10))
	require.False(t, item.thinkingExpanded)

	// Non-left button is ignored.
	require.False(t, item.HandleMouseClick(ansi.MouseRight, 0, 2))
	require.False(t, item.thinkingExpanded)
}

func TestAssistantIsSpinning(t *testing.T) {
	t.Parallel()

	now := time.Now().Unix()
	stale := time.Now().Add(-2 * summarySpinnerStallTimeout).Unix()

	tests := []struct {
		name string
		msg  message.Message
		want bool
	}{
		{
			name: "pending empty assistant message within window keeps spinning",
			msg:  message.Message{ID: "m", UpdatedAt: now, CreatedAt: now},
			want: true,
		},
		{
			name: "non-summary pending message is not subject to stall-guard",
			msg:  message.Message{ID: "m", UpdatedAt: stale, CreatedAt: stale},
			want: true,
		},
		{
			name: "has text content stops spinner",
			msg: message.Message{
				ID:        "m",
				UpdatedAt: now,
				Parts:     []message.ContentPart{message.TextContent{Text: "hello"}},
			},
			want: false,
		},
		{
			name: "has tool calls stops spinner",
			msg: message.Message{
				ID:    "m",
				Parts: []message.ContentPart{message.ToolCall{ID: "t1"}},
			},
			want: false,
		},
		{
			name: "finished normally stops spinner",
			msg: message.Message{
				ID: "m",
				Parts: []message.ContentPart{
					message.Finish{Reason: message.FinishReasonEndTurn},
				},
			},
			want: false,
		},
		{
			name: "finished with error stops spinner",
			msg: message.Message{
				ID: "m",
				Parts: []message.ContentPart{
					message.Finish{Reason: message.FinishReasonError, Message: "boom"},
				},
			},
			want: false,
		},
		{
			name: "thinking only, within window",
			msg: message.Message{
				ID:        "m",
				UpdatedAt: now,
				Parts:     []message.ContentPart{message.ReasoningContent{Thinking: "hmm"}},
			},
			want: true,
		},
		{
			name: "stale summary message trips stall-guard",
			msg: message.Message{
				ID:               "m",
				IsSummaryMessage: true,
				UpdatedAt:        stale,
				CreatedAt:        stale,
			},
			want: false,
		},
		{
			name: "summary falls back to CreatedAt when UpdatedAt is zero",
			msg: message.Message{
				ID:               "m",
				IsSummaryMessage: true,
				CreatedAt:        stale,
			},
			want: false,
		},
		{
			name: "fresh summary keeps spinning",
			msg: message.Message{
				ID:               "m",
				IsSummaryMessage: true,
				UpdatedAt:        now,
				CreatedAt:        now,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := &AssistantMessageItem{message: &tt.msg}
			if got := a.isSpinning(); got != tt.want {
				t.Fatalf("isSpinning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssistantSetMessageResetsStallLogged(t *testing.T) {
	t.Parallel()

	stale := time.Now().Add(-2 * summarySpinnerStallTimeout).Unix()
	a := &AssistantMessageItem{
		cachedMessageItem: &cachedMessageItem{},
		message: &message.Message{
			ID:               "m",
			IsSummaryMessage: true,
			UpdatedAt:        stale,
			CreatedAt:        stale,
		},
	}

	// Trip the stall-guard so the flag is set.
	_ = a.isSpinning()
	if !a.stallLogged {
		t.Fatalf("expected stallLogged=true after stale summary isSpinning call")
	}

	// A fresh update should clear the flag so a future stall can log again.
	a.SetMessage(&message.Message{
		ID:               "m",
		IsSummaryMessage: true,
		UpdatedAt:        time.Now().Unix(),
	})
	if a.stallLogged {
		t.Fatalf("expected stallLogged=false after SetMessage")
	}
}
