package axio

import "errors"

var (
	// ErrInvalidEnvironment indicates an invalid environment.
	ErrInvalidEnvironment = errors.New("invalid environment")
	// ErrInvalidLevel indicates an invalid log level.
	ErrInvalidLevel = errors.New("invalid level")
	// ErrInvalidFormat indicates an invalid encoding format.
	ErrInvalidFormat = errors.New("invalid format")
	// ErrInvalidOutputType indicates an invalid output type.
	ErrInvalidOutputType = errors.New("invalid output type")
	// ErrIncompatibleOutputs indicates that outputs are incompatible with agent mode.
	ErrIncompatibleOutputs = errors.New("outputs incompatible with agent mode")
	// ErrLoadConfig indicates failure to load configuration file.
	ErrLoadConfig = errors.New("failed to load configuration")
	// ErrUnknownFormat indicates an unknown file format.
	ErrUnknownFormat = errors.New("unknown file format")
	// ErrUnmarshalConfig indicates failure to unmarshal configuration.
	ErrUnmarshalConfig = errors.New("failed to unmarshal configuration")
	// ErrInvalidTracer indicates an invalid tracer value.
	ErrInvalidTracer = errors.New("invalid tracer")
	// ErrAuditWithoutPath indicates audit enabled without storePath.
	ErrAuditWithoutPath = errors.New("audit enabled requires storePath")
	// ErrFileOutputNoPath indicates file output without path.
	ErrFileOutputNoPath = errors.New("output type 'file' requires 'path'")
)

var (
	// ErrApplyOption indicates failure to apply a configuration option.
	ErrApplyOption = errors.New("failed to apply option")
	// ErrValidateConfig indicates that the configuration is invalid.
	ErrValidateConfig = errors.New("configuration validation failed")
	// ErrBuildOutputs indicates failure to build outputs.
	ErrBuildOutputs = errors.New("failed to build outputs")
	// ErrBuildHooks indicates failure to build hooks.
	ErrBuildHooks = errors.New("failed to build hooks")
	// ErrBuildMetrics indicates failure to build metrics.
	ErrBuildMetrics = errors.New("failed to build metrics")
	// ErrBuildEngine indicates failure to build the logging engine.
	ErrBuildEngine = errors.New("failed to build engine")
)

var (
	// ErrOpenFile indicates failure to open the log file.
	ErrOpenFile = errors.New("failed to open file")
)

var (
	// ErrLoadChainState indicates failure to load the chain state.
	ErrLoadChainState = errors.New("failed to load chain state")
	// ErrSaveChainState indicates failure to save the chain state.
	ErrSaveChainState = errors.New("failed to save chain state")
	// ErrMarshalChainState indicates failure to marshal the chain state.
	ErrMarshalChainState = errors.New("failed to marshal chain state")
	// ErrUnmarshalChainState indicates failure to unmarshal the chain state.
	ErrUnmarshalChainState = errors.New("failed to unmarshal chain state")
	// ErrHashMismatch indicates that a calculated hash does not match the expected one.
	ErrHashMismatch = errors.New("hash mismatch")
	// ErrChainBroken indicates that the chain integrity has been compromised.
	ErrChainBroken = errors.New("chain integrity compromised")
	// ErrSerializeEntry indicates failure to serialize an entry for hashing.
	ErrSerializeEntry = errors.New("failed to serialize audit entry")
	// ErrCreateAuditHook indicates failure to create the audit hook.
	ErrCreateAuditHook = errors.New("failed to create audit hook")
)

var (
	// ErrNilMetricsProvider indicates that the metrics provider is nil.
	ErrNilMetricsProvider = errors.New("metrics provider cannot be nil")
	// ErrCreateMetric indicates failure to create a metric instrument.
	ErrCreateMetric = errors.New("failed to create metric instrument")
)
