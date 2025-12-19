# WebSocket iOS Client Example

This example demonstrates how to use the Go WebSocket client to interact with a Limrun iOS instance.

## Prerequisites

- A running iOS instance (either local or remote)
- The instance's API URL and token

## Running the Example

```bash
# From the go-sdk directory
go run ./examples/websocket/main.go
```

## Features Demonstrated

- **Screenshot**: Take screenshots
- **ElementTree**: Get the accessibility hierarchy
- **ListApps**: List installed applications
- **LaunchApp**: Launch apps by bundle ID
- **OpenURL**: Open URLs in Safari or deep links
- **Tap**: Tap at coordinates
- **TapElement**: Tap elements by accessibility selector
- **TypeText**: Type text into focused fields
- **PressKey**: Press keyboard keys with optional modifiers
- **SetElementValue**: Set values on text fields (faster than typing)
- **IncrementElement/DecrementElement**: Adjust sliders and steppers
- **Lsof**: List open Unix sockets (useful for tunneling)
- **Simctl**: Run simctl commands with streaming output

## API Reference

### Creating a Client

```go
client, err := websocket.NewClient(
    "http://localhost:8833",  // API URL
    "your-token",              // Authentication token
    websocket.WithLogger(slog.Default()),  // Optional: custom logger
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### Taking Screenshots

```go
screenshot, err := client.Screenshot(ctx)
// screenshot.Base64 - base64-encoded JPEG
// screenshot.Width, screenshot.Height - dimensions in points
```

### Element Interactions

```go
// Get full element tree
tree, err := client.ElementTree(ctx, nil)

// Get element at specific point
tree, err := client.ElementTree(ctx, &websocket.AccessibilityPoint{X: 200, Y: 400})

// Tap by selector
result, err := client.TapElement(ctx, websocket.AccessibilitySelector{
    ElementType: "Button",
    Label:       "Submit",
})

// Set text field value
result, err := client.SetElementValue(ctx, "Hello", websocket.AccessibilitySelector{
    ElementType: "TextField",
})
```

### Keyboard Input

```go
// Type text
err := client.TypeText(ctx, "Hello World!", false)

// Type text and press Enter
err := client.TypeText(ctx, "search query", true)

// Press single key
err := client.PressKey(ctx, "enter")

// Press key with modifiers (variadic)
err := client.PressKey(ctx, "a", "command")           // Cmd+A
err := client.PressKey(ctx, "c", "command")           // Cmd+C
err := client.PressKey(ctx, "z", "command", "shift")  // Cmd+Shift+Z
```

### Running Simctl Commands

The Simctl API mirrors `os/exec.Cmd` for familiarity:

```go
// Capture output
output, err := client.Simctl(ctx, "listapps", "booted").Output()

// Capture combined stdout+stderr
output, err := client.Simctl(ctx, "launch", "booted", "com.example.app").CombinedOutput()

// Stream to writers
cmd := client.Simctl(ctx, "launch", "booted", "com.example.app")
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
err := cmd.Run()

// Use pipes for streaming
cmd := client.Simctl(ctx, "listapps", "booted")
stdout, _ := cmd.StdoutPipe()
cmd.Start()
io.Copy(os.Stdout, stdout)
cmd.Wait()

// With timeout (auto-kills when context expires)
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
output, err := client.Simctl(ctx, "spawn", "booted", "log", "stream").Output()

// Manual kill
cmd := client.Simctl(ctx, "spawn", "booted", "log", "stream")
cmd.Start()
time.Sleep(3 * time.Second)
cmd.Kill()  // Terminates the remote process
cmd.Wait()  // Returns with termination error
```
