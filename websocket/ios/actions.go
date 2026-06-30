package ios

import (
	"context"
	"fmt"
)

// PerformAction is a single action in a PerformActions batch. The Type field
// is the discriminator and matches the request-side message type used by the
// equivalent single-action methods. Only the fields relevant to a given Type
// are populated; the rest are omitted on the wire.
//
// Prefer the Action* constructor helpers (ActionTap, ActionTypeText, ...) to
// build values of this type.
//
// HID primitives (touchDown/touchMove/touchUp, keyDown/keyUp, buttonDown/
// buttonUp) are deliberately unpaired so callers can build their own gestures
// (e.g. long-press = touchDown + wait + touchUp).
type PerformAction struct {
	Type         string                 `json:"type"`
	X            float64                `json:"x,omitempty"`
	Y            float64                `json:"y,omitempty"`
	ScreenWidth  float64                `json:"screenWidth,omitempty"`
	ScreenHeight float64                `json:"screenHeight,omitempty"`
	Selector     *AccessibilitySelector `json:"selector,omitempty"`
	Text         string                 `json:"text,omitempty"`
	PressEnter   bool                   `json:"pressEnter,omitempty"`
	Key          string                 `json:"key,omitempty"`
	Modifiers    []string               `json:"modifiers,omitempty"`
	Direction    ScrollDirection        `json:"direction,omitempty"`
	Pixels       float64                `json:"pixels,omitempty"`
	Coordinate   []float64              `json:"coordinate,omitempty"`
	Momentum     float64                `json:"momentum,omitempty"`
	URL          string                 `json:"url,omitempty"`
	Orientation  Orientation            `json:"orientation,omitempty"`
	DurationMs   int                    `json:"durationMs,omitempty"`
	KeyCode      int                    `json:"keyCode,omitempty"`
	Button       string                 `json:"button,omitempty"`
}

// PerformActionResult is the per-action result in a PerformActions batch. Type
// identifies which action the result corresponds to; Error is non-empty when
// that action failed. Element-based actions additionally populate ElementLabel
// and ElementType on success.
type PerformActionResult struct {
	Type         string `json:"type"`
	ElementLabel string `json:"elementLabel,omitempty"`
	ElementType  string `json:"elementType,omitempty"`
	Error        string `json:"error,omitempty"`
}

// PerformActionsResult is the aggregate result of a successful PerformActions
// call. Results has one entry per submitted action.
type PerformActionsResult struct {
	Results []PerformActionResult
}

// PerformActions runs a sequence of actions sequentially inside the pod,
// without a client round-trip between steps. On the first failing action the
// batch stops early and this method returns that action's error; on full
// success it returns a Results slice with one entry per submitted action.
//
// The supplied context governs the client-side timeout. Because a batch may
// include arbitrary wait durations, use a context with a deadline that
// accounts for the total expected duration of the batch.
func (c *Client) PerformActions(ctx context.Context, actions []PerformAction) (*PerformActionsResult, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "performActions", Actions: actions})
	if err != nil {
		return nil, err
	}
	// The batch stops on the first failing action; surface that as an error
	// rather than letting callers proceed as if the whole batch succeeded.
	if err := firstActionError(resp.Results); err != nil {
		return nil, err
	}
	return &PerformActionsResult{Results: resp.Results}, nil
}

// firstActionError returns an error for the first result with a non-empty
// Error, or nil if every action succeeded.
func firstActionError(results []PerformActionResult) error {
	for _, r := range results {
		if r.Error != "" {
			return fmt.Errorf("action %q failed: %s", r.Type, r.Error)
		}
	}
	return nil
}

// ActionTap taps at the given coordinates (device's native screen dimensions).
func ActionTap(x, y float64) PerformAction {
	return PerformAction{Type: "tap", X: x, Y: y}
}

// ActionTapWithScreenSize taps at coordinates given in an explicit coordinate
// space.
func ActionTapWithScreenSize(x, y, screenWidth, screenHeight float64) PerformAction {
	return PerformAction{Type: "tap", X: x, Y: y, ScreenWidth: screenWidth, ScreenHeight: screenHeight}
}

// ActionTapElement taps an accessibility element matching the selector.
func ActionTapElement(selector AccessibilitySelector) PerformAction {
	return PerformAction{Type: "tapElement", Selector: &selector}
}

// ActionIncrementElement increments an accessibility element.
func ActionIncrementElement(selector AccessibilitySelector) PerformAction {
	return PerformAction{Type: "incrementElement", Selector: &selector}
}

// ActionDecrementElement decrements an accessibility element.
func ActionDecrementElement(selector AccessibilitySelector) PerformAction {
	return PerformAction{Type: "decrementElement", Selector: &selector}
}

// ActionSetElementValue sets the value of an accessibility element.
func ActionSetElementValue(text string, selector AccessibilitySelector) PerformAction {
	return PerformAction{Type: "setElementValue", Text: text, Selector: &selector}
}

// ActionTypeText types text into the currently focused input field.
func ActionTypeText(text string, pressEnter bool) PerformAction {
	return PerformAction{Type: "typeText", Text: text, PressEnter: pressEnter}
}

// ActionPressKey presses a key on the keyboard, optionally with modifiers.
func ActionPressKey(key string, modifiers ...string) PerformAction {
	return PerformAction{Type: "pressKey", Key: key, Modifiers: modifiers}
}

// ActionScroll scrolls in the given direction by the specified number of
// pixels. A nil opts uses the screen center with no momentum.
func ActionScroll(direction ScrollDirection, pixels float64, opts *ScrollOptions) PerformAction {
	a := PerformAction{Type: "scroll", Direction: direction, Pixels: pixels}
	if opts != nil {
		if opts.Coordinate != nil {
			a.Coordinate = []float64{opts.Coordinate[0], opts.Coordinate[1]}
		}
		a.Momentum = opts.Momentum
	}
	return a
}

// ActionToggleKeyboard toggles the on-screen software keyboard visibility.
func ActionToggleKeyboard() PerformAction {
	return PerformAction{Type: "toggleKeyboard"}
}

// ActionOpenURL opens a URL in the simulator.
func ActionOpenURL(url string) PerformAction {
	return PerformAction{Type: "openUrl", URL: url}
}

// ActionSetOrientation sets the device orientation.
func ActionSetOrientation(orientation Orientation) PerformAction {
	return PerformAction{Type: "setOrientation", Orientation: orientation}
}

// ActionWait inserts an inline delay between actions.
func ActionWait(durationMs int) PerformAction {
	return PerformAction{Type: "wait", DurationMs: durationMs}
}

// ActionTouchDown is a raw HID touch-down primitive.
func ActionTouchDown(x, y float64) PerformAction {
	return PerformAction{Type: "touchDown", X: x, Y: y}
}

// ActionTouchMove is a raw HID touch-move primitive.
func ActionTouchMove(x, y float64) PerformAction {
	return PerformAction{Type: "touchMove", X: x, Y: y}
}

// ActionTouchUp is a raw HID touch-up primitive.
func ActionTouchUp(x, y float64) PerformAction {
	return PerformAction{Type: "touchUp", X: x, Y: y}
}

// ActionKeyDown is a raw HID key-down primitive.
func ActionKeyDown(keyCode int) PerformAction {
	return PerformAction{Type: "keyDown", KeyCode: keyCode}
}

// ActionKeyUp is a raw HID key-up primitive.
func ActionKeyUp(keyCode int) PerformAction {
	return PerformAction{Type: "keyUp", KeyCode: keyCode}
}

// ActionButtonDown is a raw HID button-down primitive (e.g. "home", "lock",
// "side", "applePay", "softwareKeyboard").
func ActionButtonDown(button string) PerformAction {
	return PerformAction{Type: "buttonDown", Button: button}
}

// ActionButtonUp is a raw HID button-up primitive (e.g. "home", "lock",
// "side", "applePay", "softwareKeyboard").
func ActionButtonUp(button string) PerformAction {
	return PerformAction{Type: "buttonUp", Button: button}
}
