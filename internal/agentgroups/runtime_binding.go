package agentgroups

import "context"

type runtimeBindingKey struct{}

// Runtime choices apply to one chat execution without changing project defaults.
func WithRuntimeBinding(ctx context.Context, binding ProjectBinding) context.Context {
	return context.WithValue(ctx, runtimeBindingKey{}, binding)
}

func RuntimeBinding(ctx context.Context) (ProjectBinding, bool) {
	binding, ok := ctx.Value(runtimeBindingKey{}).(ProjectBinding)
	return binding, ok
}
