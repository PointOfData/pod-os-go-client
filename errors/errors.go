package errors

import (
	"errors"
	"fmt"
)

type ErrCode uint32

type GatewayDError struct {
	Code          ErrCode
	Message       string
	OriginalError error
}

// Error returns the error message of the GatewayDError.
// Returns a default message if the receiver is nil.
func (e *GatewayDError) Error() string {
	if e == nil {
		return "nil GatewayDError"
	}
	if e.OriginalError == nil {
		return e.Message
	}
	return fmt.Sprintf("%s, OriginalError: %s", e.Message, e.OriginalError)
}

// Wrap wraps the original error.
func (e *GatewayDError) Wrap(err error) *GatewayDError {
	return &GatewayDError{
		Code:          e.Code,
		Message:       e.Message,
		OriginalError: err,
	}
}

// Unwrap returns the original error.
// Returns nil if the receiver is nil or if OriginalError is nil.
func (e *GatewayDError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.OriginalError
}

const (
	ErrCodeUnknown ErrCode = iota
	ErrCodeNilContext
	ErrCodeClientNotFound
	ErrCodeClientNotConnected
	ErrCodeClientConnectionFailed
	ErrCodeNetworkNotSupported
	ErrCodeResolveFailed
	ErrCodePoolExhausted
	ErrCodePluginNotFound
	ErrCodePluginNotReady
	ErrCodeStartPluginFailed
	ErrCodeGetRPCClientFailed
	ErrCodeDispensePluginFailed
	ErrCodePluginMetricsMergeFailed
	ErrCodePluginPingFailed
	ErrCodePluginScaffoldFailed
	ErrCopyEmbeddedFilesFailed
	ErrCodePluginScaffoldInputFileReadFailed
	ErrCodeClientReceiveFailed
	ErrCodeClientSendFailed
	ErrCodeServerReceiveFailed
	ErrCodeServerSendFailed
	ErrCodeServerListenFailed
	ErrCodeSplitHostPortFailed
	ErrCodeAcceptFailed
	ErrCodeGetTLSConfigFailed
	ErrCodeTLSDisabled
	ErrCodeUpgradeToTLSFailed
	ErrCodeReadFailed
	ErrCodePutFailed
	ErrCodeNilPointer
	ErrCodeCastFailed
	ErrCodeHookReturnedError
	ErrCodeHookTerminatedConnection
	ErrCodeFileNotFound
	ErrCodeFileOpenFailed
	ErrCodeFileReadFailed
	ErrCodeDuplicateMetricsCollector
	ErrCodeInvalidMetricType
	ErrCodeValidationFailed
	ErrCodeLintingFailed
	ErrCodeExtractFailed
	ErrCodeDownloadFailed
	ErrCodeKeyNotFound
	ErrCodeRunError
	ErrCodeAsyncAction
	ErrCodeEvalError
	ErrCodeMsgEncodeError
	ErrCodeConfigParseError
	ErrCodePublishAsyncAction
	ErrCodeLoadBalancerStrategyNotFound
	ErrCodeNoProxiesAvailable
	ErrCodeNoLoadBalancerRules
	// ErrCodeConnectionLost marks a fatal, unrecoverable-on-this-socket condition
	// (hard I/O error, mid-frame read timeout, or framing desync). The transport
	// has been marked disconnected and the caller should reconnect/retry.
	ErrCodeConnectionLost
	// ErrCodeReceiveIdleTimeout marks a benign idle read timeout where no bytes of
	// a frame had been consumed. The connection is still considered healthy.
	ErrCodeReceiveIdleTimeout
)

var (
	ErrClientNotFound = &GatewayDError{
		ErrCodeClientNotFound, "client not found", nil,
	}
	ErrNilContext = &GatewayDError{
		ErrCodeNilContext, "context is nil", nil,
	}
	ErrClientNotConnected = &GatewayDError{
		ErrCodeClientNotConnected, "client is not connected", nil,
	}
	ErrClientConnectionFailed = &GatewayDError{
		ErrCodeClientConnectionFailed, "failed to create a new connection", nil,
	}
	ErrNetworkNotSupported = &GatewayDError{
		ErrCodeNetworkNotSupported, "network is not supported", nil,
	}
	ErrResolveFailed = &GatewayDError{
		ErrCodeResolveFailed, "failed to resolve address", nil,
	}
	ErrPoolExhausted = &GatewayDError{
		ErrCodePoolExhausted, "pool is exhausted", nil,
	}

	ErrPluginNotReady = &GatewayDError{
		ErrCodePluginNotReady, "plugin is not ready", nil,
	}
	ErrFailedToStartPlugin = &GatewayDError{
		ErrCodeStartPluginFailed, "failed to start plugin", nil,
	}
	ErrFailedToGetRPCClient = &GatewayDError{
		ErrCodeGetRPCClientFailed, "failed to get RPC client", nil,
	}
	ErrFailedToDispensePlugin = &GatewayDError{
		ErrCodeDispensePluginFailed, "failed to dispense plugin", nil,
	}
	ErrFailedToMergePluginMetrics = &GatewayDError{
		ErrCodePluginMetricsMergeFailed, "failed to merge plugin metrics", nil,
	}
	ErrFailedToPingPlugin = &GatewayDError{
		ErrCodePluginPingFailed, "failed to ping plugin", nil,
	}
	ErrFailedToScaffoldPlugin = &GatewayDError{
		ErrCodePluginScaffoldFailed, "failed to scaffold plugin", nil,
	}
	ErrFailedToCopyEmbeddedFiles = &GatewayDError{
		ErrCopyEmbeddedFilesFailed, "failed to copy embedded files", nil,
	}
	ErrFailedToReadPluginScaffoldInputFile = &GatewayDError{
		ErrCodePluginScaffoldInputFileReadFailed, "failed to read plugin scaffold input file", nil,
	}

	ErrClientReceiveFailed = &GatewayDError{
		ErrCodeClientReceiveFailed, "couldn't receive data from the server", nil,
	}
	ErrClientSendFailed = &GatewayDError{
		ErrCodeClientSendFailed, "couldn't send data to the server", nil,
	}

	ErrServerSendFailed = &GatewayDError{
		ErrCodeServerSendFailed, "couldn't send data to the client", nil,
	}
	ErrServerListenFailed = &GatewayDError{
		ErrCodeServerListenFailed, "couldn't listen on the server", nil,
	}
	ErrSplitHostPortFailed = &GatewayDError{
		ErrCodeSplitHostPortFailed, "failed to split host:port", nil,
	}
	ErrAcceptFailed = &GatewayDError{
		ErrCodeAcceptFailed, "failed to accept connection", nil,
	}
	ErrGetTLSConfigFailed = &GatewayDError{
		ErrCodeGetTLSConfigFailed, "failed to get TLS config", nil,
	}
	ErrUpgradeToTLSFailed = &GatewayDError{
		ErrCodeUpgradeToTLSFailed, "failed to upgrade to TLS", nil,
	}

	ErrReadFailed = &GatewayDError{
		ErrCodeReadFailed, "failed to read from the client", nil,
	}

	ErrNilPointer = &GatewayDError{
		ErrCodeNilPointer, "nil pointer", nil,
	}

	ErrCastFailed = &GatewayDError{
		ErrCodeCastFailed, "failed to cast", nil,
	}

	ErrHookTerminatedConnection = &GatewayDError{
		ErrCodeHookTerminatedConnection, "hook terminated connection", nil,
	}

	ErrValidationFailed = &GatewayDError{
		ErrCodeValidationFailed, "validation failed", nil,
	}
	ErrLintingFailed = &GatewayDError{
		ErrCodeLintingFailed, "linting failed", nil,
	}

	ErrExtractFailed = &GatewayDError{
		ErrCodeExtractFailed, "failed to extract the archive", nil,
	}
	ErrDownloadFailed = &GatewayDError{
		ErrCodeDownloadFailed, "failed to download the file", nil,
	}

	ErrActionNotExist = &GatewayDError{
		ErrCodeKeyNotFound, "action does not exist", nil,
	}
	ErrRunningAction = &GatewayDError{
		ErrCodeRunError, "error running action", nil,
	}
	ErrAsyncAction = &GatewayDError{
		ErrCodeAsyncAction, "async action", nil,
	}
	ErrRunningActionTimeout = &GatewayDError{
		ErrCodeRunError, "timeout running action", nil,
	}
	ErrActionNotMatched = &GatewayDError{
		ErrCodeKeyNotFound, "no matching action", nil,
	}
	ErrPolicyNotMatched = &GatewayDError{
		ErrCodeKeyNotFound, "no matching policy", nil,
	}
	ErrEvalError = &GatewayDError{
		ErrCodeEvalError, "error evaluating expression", nil,
	}
	ErrMsgEncodeError = &GatewayDError{
		ErrCodeMsgEncodeError, "error encoding message", nil,
	}

	ErrConfigParseError = &GatewayDError{
		ErrCodeConfigParseError, "error parsing config", nil,
	}
	ErrPublishingAsyncAction = &GatewayDError{
		ErrCodePublishAsyncAction, "error publishing async action", nil,
	}

	ErrLoadBalancerStrategyNotFound = &GatewayDError{
		ErrCodeLoadBalancerStrategyNotFound, "The specified load balancer strategy does not exist.", nil,
	}

	ErrNoProxiesAvailable = &GatewayDError{
		ErrCodeNoProxiesAvailable, "No proxies available to select.", nil,
	}

	ErrNoLoadBalancerRules = &GatewayDError{
		ErrCodeNoLoadBalancerRules, "No load balancer rules provided.", nil,
	}

	// ErrConnectionLost is a fatal transport error: the socket is dead and must be
	// reconnected. Returned for hard I/O errors, mid-frame read timeouts, and
	// framing desync. Detect with IsConnectionLost.
	ErrConnectionLost = &GatewayDError{
		ErrCodeConnectionLost, "connection lost", nil,
	}
	// ErrReceiveIdleTimeout is a benign read timeout: no frame bytes were pending,
	// so the connection is still considered alive. Detect with IsIdleTimeout.
	ErrReceiveIdleTimeout = &GatewayDError{
		ErrCodeReceiveIdleTimeout, "receive idle timeout", nil,
	}

	// Unwrapped errors.
	ErrLoggerRequired = errors.New("terminate action requires a logger parameter")
)

// codeOf extracts the ErrCode from any error in the chain that is a *GatewayDError.
func codeOf(err error) (ErrCode, bool) {
	var ge *GatewayDError
	if errors.As(err, &ge) && ge != nil {
		return ge.Code, true
	}
	return ErrCodeUnknown, false
}

// IsConnectionLost reports whether err (or any wrapped error) is a fatal
// connection-lost transport error.
func IsConnectionLost(err error) bool {
	code, ok := codeOf(err)
	return ok && code == ErrCodeConnectionLost
}

// IsIdleTimeout reports whether err (or any wrapped error) is a benign idle
// receive timeout (the connection is still healthy).
func IsIdleTimeout(err error) bool {
	code, ok := codeOf(err)
	return ok && code == ErrCodeReceiveIdleTimeout
}
