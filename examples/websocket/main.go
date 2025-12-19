package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	websocket "github.com/limrun-inc/go-sdk/websocket/ios"
)

func main() {
	ctx := context.Background()

	// Connect to the iOS instance
	client, err := websocket.NewClient(
		"http://localhost:8833",
		"your-token",
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// ========================================================================
	// Screenshot
	// ========================================================================
	fmt.Println("\n--- Testing Screenshot ---")
	screenshot, err := client.Screenshot(ctx)
	if err != nil {
		log.Fatalf("Failed to take screenshot: %v", err)
	}
	fmt.Printf("Screenshot taken: %.0fx%.0f, %d bytes base64\n",
		screenshot.Width, screenshot.Height, len(screenshot.Base64))

	// ========================================================================
	// Element Tree (Accessibility hierarchy)
	// ========================================================================
	fmt.Println("\n--- Testing ElementTree ---")
	tree, err := client.ElementTree(ctx, nil)
	if err != nil {
		log.Printf("Failed to get element tree: %v", err)
	} else {
		fmt.Printf("Element tree received: %d characters\n", len(tree))
	}

	// Element tree at specific point
	fmt.Println("\n--- Testing ElementTree at point ---")
	pointTree, err := client.ElementTree(ctx, &websocket.AccessibilityPoint{X: 200, Y: 400})
	if err != nil {
		log.Printf("Failed to get element tree at point: %v", err)
	} else {
		fmt.Printf("Element tree at point: %d characters\n", len(pointTree))
	}

	// ========================================================================
	// List Apps
	// ========================================================================
	fmt.Println("\n--- Testing ListApps ---")
	apps, err := client.ListApps(ctx)
	if err != nil {
		log.Printf("Failed to list apps: %v", err)
	} else {
		fmt.Printf("Found %d installed apps:\n", len(apps))
		for i, app := range apps {
			if i >= 5 {
				fmt.Printf("  ... and %d more\n", len(apps)-5)
				break
			}
			fmt.Printf("  - %s (%s) [%s]\n", app.Name, app.BundleID, app.InstallType)
		}
	}

	// ========================================================================
	// Launch App (Safari)
	// ========================================================================
	fmt.Println("\n--- Testing LaunchApp ---")
	if err := client.LaunchApp(ctx, "com.apple.mobilesafari"); err != nil {
		log.Printf("Failed to launch Safari: %v", err)
	} else {
		fmt.Println("Launched Safari")
	}
	time.Sleep(1 * time.Second)

	// ========================================================================
	// Open URL
	// ========================================================================
	fmt.Println("\n--- Testing OpenURL ---")
	if err := client.OpenURL(ctx, "https://www.example.com"); err != nil {
		log.Printf("Failed to open URL: %v", err)
	} else {
		fmt.Println("Opened URL: https://www.example.com")
	}
	time.Sleep(2 * time.Second)

	// ========================================================================
	// Tap at coordinates
	// ========================================================================
	fmt.Println("\n--- Testing Tap ---")
	if err := client.Tap(ctx, screenshot.Width/2, screenshot.Height/2); err != nil {
		log.Printf("Failed to tap: %v", err)
	} else {
		fmt.Printf("Tapped at center: (%.0f, %.0f)\n", screenshot.Width/2, screenshot.Height/2)
	}
	time.Sleep(500 * time.Millisecond)

	// ========================================================================
	// Tap Element by selector
	// ========================================================================
	fmt.Println("\n--- Testing TapElement ---")
	result, err := client.TapElement(ctx, websocket.AccessibilitySelector{
		ElementType: "TextField",
	})
	if err != nil {
		fmt.Printf("TapElement failed (expected if no matching element): %v\n", err)
	} else {
		fmt.Printf("Tapped element: %s - \"%s\"\n", result.ElementType, result.ElementLabel)
	}
	time.Sleep(500 * time.Millisecond)

	// ========================================================================
	// Type Text
	// ========================================================================
	fmt.Println("\n--- Testing TypeText ---")
	if err := client.TypeText(ctx, "Hello from Go SDK!", false); err != nil {
		log.Printf("Failed to type text: %v", err)
	} else {
		fmt.Println("Typed text: \"Hello from Go SDK!\"")
	}
	time.Sleep(500 * time.Millisecond)

	// ========================================================================
	// Press Key
	// ========================================================================
	fmt.Println("\n--- Testing PressKey ---")
	if err := client.PressKey(ctx, "enter"); err != nil {
		log.Printf("Failed to press Enter: %v", err)
	} else {
		fmt.Println("Pressed Enter key")
	}
	time.Sleep(500 * time.Millisecond)

	// Press with modifiers (Command+A to select all)
	if err := client.PressKey(ctx, "a", "command"); err != nil {
		log.Printf("Failed to press Command+A: %v", err)
	} else {
		fmt.Println("Pressed Command+A")
	}
	time.Sleep(500 * time.Millisecond)

	// ========================================================================
	// Set Element Value (faster than typing)
	// ========================================================================
	fmt.Println("\n--- Testing SetElementValue ---")
	elemResult, err := client.SetElementValue(ctx, "https://apple.com", websocket.AccessibilitySelector{
		ElementType: "TextField",
	})
	if err != nil {
		fmt.Printf("SetElementValue failed (expected if no matching element): %v\n", err)
	} else {
		fmt.Printf("Set value on element: \"%s\"\n", elemResult.ElementLabel)
	}

	// ========================================================================
	// Increment/Decrement Element (for sliders, steppers)
	// ========================================================================
	fmt.Println("\n--- Testing IncrementElement/DecrementElement ---")
	_, err = client.IncrementElement(ctx, websocket.AccessibilitySelector{
		ElementType: "Slider",
	})
	if err != nil {
		fmt.Printf("IncrementElement failed (expected if no slider): %v\n", err)
	}

	_, err = client.DecrementElement(ctx, websocket.AccessibilitySelector{
		ElementType: "Slider",
	})
	if err != nil {
		fmt.Printf("DecrementElement failed (expected if no slider): %v\n", err)
	}

	// ========================================================================
	// List Open Files (Unix sockets)
	// ========================================================================
	fmt.Println("\n--- Testing Lsof ---")
	files, err := client.Lsof(ctx)
	if err != nil {
		log.Printf("Failed to list open files: %v", err)
	} else {
		fmt.Printf("Found %d open unix sockets:\n", len(files))
		for i, file := range files {
			if i >= 5 {
				fmt.Printf("  ... and %d more\n", len(files)-5)
				break
			}
			fmt.Printf("  - [%s] %s\n", file.Kind, file.Path)
		}
	}

	// ========================================================================
	// Simctl (like os/exec.Cmd)
	// ========================================================================
	fmt.Println("\n--- Testing Simctl ---")

	// Method 1: Capture output (like exec.Command().Output())
	output, err := client.Simctl(ctx, "listapps", "booted").Output()
	if err != nil {
		log.Printf("Simctl listapps failed: %v", err)
	} else {
		lines := strings.Split(string(output), "\n")
		fmt.Printf("Simctl output (%d lines):\n", len(lines))
		for i, line := range lines {
			if i >= 3 {
				fmt.Printf("  ... and %d more lines\n", len(lines)-3)
				break
			}
			if len(line) > 80 {
				line = line[:80] + "..."
			}
			fmt.Printf("  %s\n", line)
		}
	}

	// Method 2: Stream output with timeout (auto-kills when context expires)
	fmt.Println("\n--- Testing Simctl with streaming (log stream for 3s) ---")
	streamCtx, streamCancel := context.WithTimeout(ctx, 3*time.Second)
	defer streamCancel()

	cmd := client.Simctl(streamCtx, "spawn", "booted", "log", "stream", "--style", "compact")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to get stdout pipe: %v", err)
	} else {
		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start log stream: %v", err)
		} else {
			lineCount := 0

			// Read lines in a goroutine
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := stdout.Read(buf)
					if err != nil {
						break
					}
					// Count newlines in the chunk
					for _, b := range buf[:n] {
						if b == '\n' {
							lineCount++
						}
					}
				}
			}()

			// Wait will return when context times out and process is killed
			if err := cmd.Wait(); err != nil {
				fmt.Printf("Process terminated after 3s timeout: %v (received %d log lines)\n", err, lineCount)
			}
		}
	}

	fmt.Println("\n✅ All tests completed!")
}
