package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	api := limrun.NewClient()
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "websocket-ios-example"}},
	})
	if err != nil {
		return err
	}
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	screenshot, err := client.Screenshot(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Screenshot: %gx%g\n", screenshot.Width, screenshot.Height)
	if err := exampleutil.SaveBase64("screenshot.jpg", screenshot.Base64); err != nil {
		return err
	}
	tree, err := client.ElementTreeNodes(ctx, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Accessibility tree: %d roots\n", len(tree))

	// A reused instance might have an app in front: go home before finding Safari.
	if _, err := client.PerformActions(ctx, []ios.PerformAction{{Type: "buttonDown", Button: "home"}, {Type: "buttonUp", Button: "home"}}); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, time.Second); err != nil {
		return err
	}
	tap := func(selector ios.AccessibilitySelector, timeout time.Duration) error {
		return exampleutil.Retry(ctx, timeout, time.Second, func(ctx context.Context) error {
			element, err := client.TapElement(ctx, selector)
			if err == nil {
				fmt.Printf("Tapped %s: %s\n", element.ElementType, element.ElementLabel)
			}
			return err
		})
	}
	if err := tap(ios.AccessibilitySelector{AccessibilityID: "Safari"}, 20*time.Second); err != nil {
		return err
	}
	textField := ios.AccessibilitySelector{ElementType: "TextField"}
	if err := tap(textField, 90*time.Second); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, 3*time.Second); err != nil {
		return err
	}
	if err := exampleutil.Retry(ctx, time.Minute, 2*time.Second, func(ctx context.Context) error {
		if err := client.TypeText(ctx, "Hello from Go SDK!", false); err != nil {
			// The address bar can lose focus during its opening animation.
			_, _ = client.TapElement(ctx, textField)
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		if err := client.ToggleKeyboard(ctx); err != nil {
			return err
		}
		if err := exampleutil.Sleep(ctx, time.Second); err != nil {
			return err
		}
	}
	if err := client.PressKey(ctx, "enter"); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, 3*time.Second); err != nil {
		return err
	}
	if err := tap(ios.AccessibilitySelector{ElementType: "Button", Label: "Close"}, 10*time.Second); err != nil {
		fmt.Println("No Close overlay to dismiss")
	}
	if err := exampleutil.Sleep(ctx, 5*time.Second); err != nil {
		return err
	}
	if _, err := client.TapElement(ctx, textField); err != nil {
		fmt.Printf("No address field: %v\n", err)
	} else {
		if err := exampleutil.Sleep(ctx, 2*time.Second); err != nil {
			return err
		}
		if _, err := client.SetElementValue(ctx, "https://apple.com", textField); err != nil {
			fmt.Printf("setElementValue: %v\n", err)
		} else if err := client.PressKey(ctx, "enter"); err != nil {
			return err
		}
	}
	if err := exampleutil.Sleep(ctx, 3*time.Second); err != nil {
		return err
	}

	// SimctlCmd follows os/exec.Cmd: assign writers, then Run and check ExitCode.
	command := client.Simctl(ctx, "listapps", "booted")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return err
	}
	fmt.Printf("simctl exited: %d\n", command.ExitCode())

	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return err
	}
	cx, startY := info.ScreenWidth/2, info.ScreenHeight/2+100
	endY := startY - 200
	var actions []ios.PerformAction
	// The same raw HID gesture as the TypeScript example: 200 points up and back.
	for _, stroke := range [][2]float64{{startY, endY}, {endY, startY}} {
		actions = append(actions, ios.PerformAction{Type: "touchDown", X: cx, Y: stroke[0]})
		for step := 1; step <= 4; step++ {
			actions = append(actions, ios.PerformAction{Type: "wait", DurationMs: 30}, ios.PerformAction{Type: "touchMove", X: cx, Y: stroke[0] + (stroke[1]-stroke[0])*float64(step)/4})
		}
		actions = append(actions, ios.PerformAction{Type: "touchUp", X: cx, Y: stroke[1]})
		if stroke[0] == startY {
			actions = append(actions, ios.PerformAction{Type: "wait", DurationMs: 800})
		}
	}
	if result, err := client.PerformActions(ctx, actions, ios.PerformActionsOptions{Timeout: 10 * time.Second}); err != nil {
		fmt.Printf("HID batch: %v\n", err)
	} else {
		fmt.Printf("Executed %d HID events\n", len(result.Results))
	}
	tree, err = client.ElementTreeNodes(ctx, nil)
	if err != nil {
		return err
	}
	printWebContent(tree)
	reset, err := client.SoftReset(ctx, "com.apple.mobilesafari", &ios.SoftResetOptions{Strategy: "data"})
	if err != nil {
		return err
	}
	fmt.Printf("Soft reset: %+v\n", reset)
	if err := exampleutil.Sleep(ctx, 3*time.Second); err != nil {
		return err
	}
	final, err := client.Screenshot(ctx)
	if err != nil {
		return err
	}
	if err := exampleutil.SaveBase64("screenshot-final.jpg", final.Base64); err != nil {
		return err
	}
	fmt.Printf("Instance %s remains available.\n", instance.Metadata.ID)
	return nil
}

func printWebContent(nodes []ios.ElementTreeNode) {
	for _, node := range nodes {
		if slices.Contains(node.Traits, "WebContent") {
			label, id := "", ""
			if node.AXLabel != nil {
				label = *node.AXLabel
			}
			if node.AXUniqueID != nil {
				id = *node.AXUniqueID
			}
			fmt.Printf("WebContent: type=%s role=%s label=%q id=%q\n", node.Type, node.Role, label, id)
		}
		printWebContent(node.Children)
	}
}
