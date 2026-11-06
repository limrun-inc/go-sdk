package limrun

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"

	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/websocket/android"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

func instanceCredentials(apiURL, token string) error {
	if apiURL == "" {
		return errors.New("instance not ready: apiUrl is not available")
	}
	if token == "" {
		return errors.New("instance control token is not available; the caller needs full control of the instance")
	}
	return nil
}
func (r *IosInstanceService) CreateClient(instance IosInstance, opts ...ios.Option) (*ios.Client, error) {
	if err := instanceCredentials(instance.Status.APIURL, instance.Status.Token); err != nil {
		return nil, err
	}
	return ios.NewClient(instance.Status.APIURL, instance.Status.Token, opts...)
}
func (r *AndroidInstanceService) CreateClient(instance AndroidInstance, opts ...android.Option) (*android.Client, error) {
	if err := instanceCredentials(instance.Status.APIURL, instance.Status.Token); err != nil {
		return nil, err
	}
	opts = append([]android.Option{android.WithADBURL(instance.Status.AdbWebSocketURL)}, opts...)
	return android.NewClient(instance.Status.APIURL, instance.Status.Token, opts...)
}

type XcodeClient struct {
	*build.XcodeClient
	ios IosInstanceService
}

func (r *XcodeInstanceService) CreateClient(instance XcodeInstance, opts ...build.Option) (*XcodeClient, error) {
	if err := instanceCredentials(instance.Status.APIURL, instance.Status.Token); err != nil {
		return nil, err
	}
	return r.CreateClientWithCredentials(instance.Status.APIURL, instance.Status.Token, opts...)
}
func (r *GradleInstanceService) CreateClient(instance GradleInstance, opts ...build.Option) (*build.GradleClient, error) {
	if err := instanceCredentials(instance.Status.APIURL, instance.Status.Token); err != nil {
		return nil, err
	}
	return build.NewGradleClient(instance.Status.APIURL, instance.Status.Token, opts...), nil
}

type AttachedSimulator struct {
	IosInstanceID string
	Simulator     *IosInstance
}

func (c *XcodeClient) AttachNewSimulator(ctx context.Context, device *IosInstanceNewParamsSpec) (*AttachedSimulator, error) {
	var spec IosInstanceNewParamsSpec
	if device != nil {
		spec.Model = device.Model
		spec.Clues = device.Clues
	}
	spec.Role = IosInstanceNewParamsSpecRole{Permissions: []string{}}
	instance, err := c.ios.New(ctx, IosInstanceNewParams{Wait: Bool(true), Spec: spec})
	if err != nil {
		return nil, err
	}
	if _, err = c.AttachSimulator(ctx, instance.Status.APIURL, instance.Status.Token); err != nil {
		_ = c.ios.Delete(context.Background(), instance.Metadata.ID)
		return nil, err
	}
	return &AttachedSimulator{instance.Metadata.ID, instance}, nil
}
func (c *XcodeClient) DeleteSimulator(ctx context.Context, id string) bool {
	return c.ios.Delete(ctx, id) == nil
}
func (r *XcodeInstanceService) ListBuildLogs(ctx context.Context, id string, opts ...option.RequestOption) (res []build.BuildLog, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/xcode_instances/"+url.PathEscape(id)+"/build_logs", nil, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *GradleInstanceService) ListBuildLogs(ctx context.Context, id string, opts ...option.RequestOption) (res []build.BuildLog, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/gradle_instances/"+url.PathEscape(id)+"/build_logs", nil, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *XcodeInstanceService) ListBazelBuildLogs(ctx context.Context, id string, opts ...option.RequestOption) (res []build.BazelBuildLog, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/xcode_instances/"+url.PathEscape(id)+"/bazel_build_logs", nil, &res, slices.Concat(r.Options, opts)...)
	return
}

// CreateClientWithCredentials connects to a build daemon using an API URL and instance token.
func (r *XcodeInstanceService) CreateClientWithCredentials(apiURL, token string, opts ...build.Option) (*XcodeClient, error) {
	if err := instanceCredentials(apiURL, token); err != nil {
		return nil, err
	}
	return &XcodeClient{build.NewXcodeClient(apiURL, token, opts...), NewIosInstanceService(r.Options...)}, nil
}
