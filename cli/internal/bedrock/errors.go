package bedrock

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Sentinel errors for application-level error handling.
var (
	ErrThrottled       = errors.New("bedrock: request throttled")
	ErrModelTimeout    = errors.New("bedrock: model timeout")
	ErrServiceUnavail  = errors.New("bedrock: service unavailable")
	ErrAccessDenied    = errors.New("bedrock: access denied")
	ErrSSOExpired      = errors.New("bedrock: SSO credentials expired — run `aws sso login`")
	ErrMalformedOutput = errors.New("bedrock: malformed model output")
)

// classifyError maps AWS SDK errors to sentinel errors for retry and display decisions.
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	// SSO credentials expired
	var ssoErr *ssocreds.InvalidTokenError
	if errors.As(err, &ssoErr) {
		return fmt.Errorf("%w: %s", ErrSSOExpired, ssoErr.Error())
	}

	// Bedrock-specific error types
	var throttle *types.ThrottlingException
	if errors.As(err, &throttle) {
		return fmt.Errorf("%w: %s", ErrThrottled, throttle.ErrorMessage())
	}

	var timeout *types.ModelTimeoutException
	if errors.As(err, &timeout) {
		return fmt.Errorf("%w: %s", ErrModelTimeout, timeout.ErrorMessage())
	}

	var svcUnavail *types.ServiceUnavailableException
	if errors.As(err, &svcUnavail) {
		return fmt.Errorf("%w: %s", ErrServiceUnavail, svcUnavail.ErrorMessage())
	}

	var accessDenied *types.AccessDeniedException
	if errors.As(err, &accessDenied) {
		return fmt.Errorf("%w: %s", ErrAccessDenied, accessDenied.ErrorMessage())
	}

	var internal *types.InternalServerException
	if errors.As(err, &internal) {
		return fmt.Errorf("%w: %s", ErrServiceUnavail, internal.ErrorMessage())
	}

	var notFound *types.ResourceNotFoundException
	if errors.As(err, &notFound) {
		return fmt.Errorf("bedrock: resource not found: %s", notFound.ErrorMessage())
	}

	var validationErr *types.ValidationException
	if errors.As(err, &validationErr) {
		return fmt.Errorf("bedrock: validation error: %s", validationErr.ErrorMessage())
	}

	var modelErr *types.ModelErrorException
	if errors.As(err, &modelErr) {
		return fmt.Errorf("bedrock: model error: %s", modelErr.ErrorMessage())
	}

	var modelNotReady *types.ModelNotReadyException
	if errors.As(err, &modelNotReady) {
		return fmt.Errorf("%w: %s", ErrServiceUnavail, modelNotReady.ErrorMessage())
	}

	var modelStreamErr *types.ModelStreamErrorException
	if errors.As(err, &modelStreamErr) {
		return fmt.Errorf("bedrock: model stream error: %s", modelStreamErr.ErrorMessage())
	}

	var serviceQuota *types.ServiceQuotaExceededException
	if errors.As(err, &serviceQuota) {
		return fmt.Errorf("%w: %s", ErrThrottled, serviceQuota.ErrorMessage())
	}

	return err
}

// isRetryable returns true if the classified error should trigger an application-level retry.
// SDK-level retries handle throttling, service unavailability, and network errors.
// Application-level retry handles malformed output and model timeout.
func isRetryable(err error) bool {
	return errors.Is(err, ErrModelTimeout) || errors.Is(err, ErrMalformedOutput)
}
