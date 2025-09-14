package hive

// DefaultMailboxSize is the default capacity for an actor's message channel (mailbox).
// This value is used when a new actor process is created.
const DefaultMailboxSize = 64

// Options holds configuration settings for an actor type.
type Options struct {
	// MailboxSize specifies the capacity of an actor's message channel.
	// If not set, DefaultMailboxSize is used.
	MailboxSize int
}

// Option is a function that applies a configuration option to Options.
type Option func(*Options)

// WithMailboxSize returns an Option to set the actor's mailbox size.
// It panics if the size is not a positive integer.
func WithMailboxSize(size int) Option {
	if size <= 0 {
		panic("hive.WithMailboxSize: mailbox size must be positive")
	}

	return func(opts *Options) {
		opts.MailboxSize = size
	}
}

// defaultOptions returns a new Options struct with default values.
func defaultOptions() Options {
	return Options{
		MailboxSize: DefaultMailboxSize,
	}
}
