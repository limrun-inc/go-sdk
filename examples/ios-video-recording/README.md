# Record an iOS simulator

Records a visit to Settings → General and apple.com, then downloads `video.mp4` to the repository root. It closes the client and deletes the instance on completion or interruption.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
go run ./examples/ios-video-recording
```

The key calls are `StartRecording(ctx, nil)` and `StopRecording(ctx, ios.SaveRecordingTo{LocalPath: "video.mp4"})`. Let the scenario finish to save the video.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/ios-video-recording) · [All examples](../README.md)
