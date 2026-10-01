package execution

import (
	"context"
	"fmt"
	"io"
)

type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
)

const (
	LifecycleInterruptDelivered = "interrupt-delivered"
	LifecycleGraceExpired       = "grace-expired"
	LifecycleForcedKill         = "forced-kill"
	LifecycleOutputLimit        = "output-limit"
)

type StreamSink interface {
	Emit(Stream, []byte) error
}

type LifecycleSink interface {
	RecordLifecycle(string)
}

type streamSinkKey struct{}
type lifecycleSinkKey struct{}

func WithStreamSink(ctx context.Context, sink StreamSink) context.Context {
	if ctx == nil || sink == nil {
		return ctx
	}
	return context.WithValue(ctx, streamSinkKey{}, sink)
}

func WithLifecycleSink(ctx context.Context, sink LifecycleSink) context.Context {
	if ctx == nil || sink == nil {
		return ctx
	}
	return context.WithValue(ctx, lifecycleSinkKey{}, sink)
}

func streamSinkFrom(ctx context.Context) StreamSink {
	if ctx == nil {
		return nil
	}
	sink, _ := ctx.Value(streamSinkKey{}).(StreamSink)
	return sink
}

func recordLifecycle(ctx context.Context, event string) {
	if ctx == nil {
		return
	}
	if sink, ok := ctx.Value(lifecycleSinkKey{}).(LifecycleSink); ok {
		sink.RecordLifecycle(event)
	}
}

type sinkWriter struct {
	sink   StreamSink
	stream Stream
	chunk  int
}

func (w sinkWriter) Write(p []byte) (int, error) {
	if w.sink == nil || w.chunk <= 0 {
		return 0, fmt.Errorf("execution: invalid stream sink")
	}
	written := 0
	for len(p) > 0 {
		size := len(p)
		if size > w.chunk {
			size = w.chunk
		}
		part := append([]byte(nil), p[:size]...)
		if err := w.sink.Emit(w.stream, part); err != nil {
			return written, err
		}
		written += size
		p = p[size:]
	}
	return written, nil
}

func streamWriter(sink StreamSink, stream Stream, chunkBytes int) io.Writer {
	return sinkWriter{sink: sink, stream: stream, chunk: chunkBytes}
}
