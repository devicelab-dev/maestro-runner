package devicelab

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// droppingField simulates an EditText that loses the first character of the
// first write, the way a janky frame drops a keystroke. Its element is built
// like the DeviceLab adapter builds one: text cached from the find, writes
// through callbacks, and a text func that reads the device.
type droppingField struct {
	value   string
	writes  []string
	dropped bool
}

func (f *droppingField) write(text string) {
	f.writes = append(f.writes, text)
	if !f.dropped && len(text) > 1 {
		f.dropped = true
		text = text[1:]
	}
	f.value += text
}

func (f *droppingField) element(cachedHint string) *uiautomator2.Element {
	e := uiautomator2.NewCachedElement("e1", cachedHint, uiautomator2.ElementRect{Width: 100, Height: 50})
	e.SetSendKeysFunc(func(text string) error { f.write(text); return nil })
	e.SetClearFunc(func() error { f.value = ""; return nil })
	e.SetTextFunc(func() (string, error) { return f.value, nil })
	return e
}

// keyEventClient routes raw key events into the dropping field, as the device
// delivers them to whatever holds focus.
type keyEventClient struct {
	*scriptedClient
	field *droppingField
}

func (k *keyEventClient) SendKeyActions(text string) error {
	k.field.write(text)
	return k.scriptedClient.SendKeyActions(text)
}

// The cached text of a DeviceLab element is the value from the find. Reading it
// back after typing must reach the device, or a dropped character reads as
// "unchanged" and is never retyped.
func TestInputText_RetypesDroppedCharacters(t *testing.T) {
	tests := []struct {
		name string
		run  func(field *droppingField) *core.CommandResult
	}{
		{"selector", func(field *droppingField) *core.CommandResult {
			client := &scriptedClient{trackingClient: newTrackingClient()}
			client.findElementReturn = field.element("Username")
			return New(client, &core.PlatformInfo{}, &mockShell{}).inputText(&flow.InputTextStep{
				Selector: flow.Selector{ID: "username"}, Text: "bob",
			})
		}},
		{"focused field", func(field *droppingField) *core.CommandResult {
			client := &scriptedClient{trackingClient: newTrackingClient()}
			client.activeElementReturn = field.element("Username")
			return New(client, &core.PlatformInfo{}, &mockShell{}).inputText(&flow.InputTextStep{Text: "bob"})
		}},
		{"key press", func(field *droppingField) *core.CommandResult {
			client := &keyEventClient{scriptedClient: &scriptedClient{trackingClient: newTrackingClient()}, field: field}
			client.activeElementReturn = field.element("Username")
			return New(client, &core.PlatformInfo{}, &mockShell{}).inputText(&flow.InputTextStep{Text: "bob", KeyPress: true})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := &droppingField{}
			res := tt.run(field)
			if !res.Success {
				t.Fatalf("inputText: %v", res.Error)
			}
			if field.value != "bob" {
				t.Errorf("field holds %q, want %q (writes: %q)", field.value, "bob", field.writes)
			}
			if !strings.Contains(res.Message, "retyped") {
				t.Errorf("result %q does not report the retype", res.Message)
			}
		})
	}
}

func TestInvalidateText(t *testing.T) {
	field := &droppingField{value: "fresh"}
	native := &NativeElement{elem: field.element("stale")}

	tests := []struct {
		name  string
		field core.TextField
		want  string
	}{
		{"native element re-reads", native, "fresh"},
		{"nil field is ignored", nil, ""},
		{"field without a cache is ignored", core.TextFieldFuncs(func() (string, error) { return "live", nil }, nil, nil), "live"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalidateText(tt.field)
			if tt.field == nil {
				return
			}
			if got, _ := tt.field.Text(); got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}
