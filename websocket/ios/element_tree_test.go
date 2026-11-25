package ios

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReadElementTreeReportsKeyboardModalAndIdle(t *testing.T) {
	requests := make(chan map[string]json.RawMessage, 2)
	c := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		requests <- req
		reply := map[string]any{"id": field(req, "id"), "type": "elementTreeResult", "json": `[{"type":"Application"}]`}
		if field(req, "source") == "xcuitest" {
			reply["modal"] = map[string]any{"kind": "app", "label": "Delete note?"}
		} else {
			reply["keyboard"] = nil
			reply["idle"] = true
		}
		_ = ws.WriteJSON(reply)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	read, err := c.ReadElementTree(ctx, nil, ElementTreeOptions{WaitForIdleTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if req := <-requests; string(req["waitForIdleTimeoutMs"]) != "2000" {
		t.Fatalf("waitForIdleTimeoutMs = %s, want 2000", req["waitForIdleTimeoutMs"])
	}
	if len(read.Tree) != 1 || !read.KeyboardReported || read.Keyboard != nil || read.ModalReported || read.Idle == nil || !*read.Idle {
		t.Fatalf("accessibility read = %+v, want one node, a reported hidden keyboard, no modal and idle", read)
	}

	read, err = c.ReadElementTree(ctx, nil, ElementTreeOptions{Source: ElementTreeSourceXCUITest})
	if err != nil {
		t.Fatal(err)
	}
	if req := <-requests; field(req, "source") != "xcuitest" {
		t.Fatalf("source = %s, want xcuitest", req["source"])
	}
	if read.KeyboardReported || !read.ModalReported || read.Modal == nil || read.Modal.Kind != "app" || read.Idle != nil {
		t.Fatalf("xcuitest read = %+v, want an app modal and nothing else", read)
	}
}

func TestFocusedElementUnderTheKeyboard(t *testing.T) {
	c := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		_ = ws.WriteJSON(map[string]any{
			"id": field(req, "id"), "type": "focusedElementResult", "json": nil,
			"keyboard": map[string]any{"frame": map[string]any{"x": 0, "y": 546, "width": 402, "height": 328}},
		})
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	focus, err := c.FocusedElement(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if focus.Element != nil || focus.Keyboard == nil || focus.Keyboard.Frame.Y != 546 {
		t.Fatalf("focus = %+v, want no element and the keyboard at y 546", focus)
	}
}

func TestFailedCommandsCarryCodeAndWritten(t *testing.T) {
	c := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		reply := map[string]any{"id": field(req, "id"), "type": field(req, "type") + "Result"}
		if field(req, "type") == "performActions" {
			reply["error"] = "No focused accessibility element found"
			reply["results"] = []map[string]any{
				{"type": "tap"},
				{"type": "typeText", "error": "No focused accessibility element found", "code": "no_focused_element", "written": "none"},
			}
		} else {
			reply["error"] = "No focused accessibility element found"
			reply["code"] = "no_focused_element"
			reply["written"] = "none"
		}
		_ = ws.WriteJSON(reply)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for name, run := range map[string]func() error{
		"typeText": func() error { return c.TypeText(ctx, "hello", false) },
		"performActions": func() error {
			_, err := c.PerformActions(ctx, []PerformAction{ActionTap(1, 2), ActionTypeText("hello", false)})
			return err
		},
	} {
		var rejected *CommandError
		if err := run(); !errors.As(err, &rejected) {
			t.Fatalf("%s error = %v, want a CommandError", name, err)
		}
		if rejected.Message != "No focused accessibility element found" || rejected.Code != "no_focused_element" || rejected.Written != WrittenNone {
			t.Fatalf("%s error = %+v, want the message, no_focused_element and none", name, rejected)
		}
	}
}
