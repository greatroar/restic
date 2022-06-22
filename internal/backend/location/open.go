// Package backends knows how to open or create all the supported backends.
//
// This is a separate package to break import cycles.
package location

import (
	"context"
	"net/http"
	"os"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/azure"
	"github.com/restic/restic/internal/backend/b2"
	"github.com/restic/restic/internal/backend/gs"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/backend/swift"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/limiter"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"
)

// Create makes a new backend at the storage location designated by s.
func Create(ctx context.Context, s string, opts options.Options, tropts backend.TransportOptions) (restic.Backend, error) {
	debug.Log("parsing location %v", StripPassword(s))
	loc, err := parseLocation(s)
	if err != nil {
		return nil, err
	}

	cfg, err := parseConfig(loc, opts)
	if err != nil {
		return nil, err
	}

	rt, err := backend.Transport(tropts)
	if err != nil {
		return nil, err
	}

	switch loc.Scheme {
	case "local":
		return local.Create(ctx, cfg.(local.Config))
	case "sftp":
		return sftp.Create(ctx, cfg.(sftp.Config))
	case "s3":
		return s3.Create(ctx, cfg.(s3.Config), rt)
	case "gs":
		return gs.Create(cfg.(gs.Config), rt)
	case "azure":
		return azure.Create(cfg.(azure.Config), rt)
	case "swift":
		return swift.Open(ctx, cfg.(swift.Config), rt)
	case "b2":
		return b2.Create(ctx, cfg.(b2.Config), rt)
	case "rest":
		return rest.Create(ctx, cfg.(rest.Config), rt)
	case "rclone":
		return rclone.Create(ctx, cfg.(rclone.Config))
	}

	debug.Log("invalid repository scheme: %v", s)
	return nil, errors.Fatalf("invalid scheme %q", loc.Scheme)
}

// Open connects to the backend at the storage location designated by s.
func Open(ctx context.Context, s string, opts options.Options, tropts backend.TransportOptions, limits limiter.Limits) (restic.Backend, error) {
	debug.Log("parsing location %v", StripPassword(s))
	loc, err := parseLocation(s)
	if err != nil {
		return nil, errors.Fatalf("parsing repository location failed: %v", err)
	}

	var be restic.Backend

	cfg, err := parseConfig(loc, opts)
	if err != nil {
		return nil, err
	}

	usesHTTP := loc.Scheme != "local" && loc.Scheme != "sftp"

	lim := limiter.NewStaticLimiter(limits)
	var rt http.RoundTripper

	if usesHTTP {
		rt, err = backend.Transport(tropts)
		if err != nil {
			return nil, err
		}

		// wrap the transport so that the throughput via HTTP is limited
		rt = lim.Transport(rt)
	}

	switch loc.Scheme {
	case "local":
		be, err = local.Open(ctx, cfg.(local.Config))
	case "sftp":
		be, err = sftp.Open(ctx, cfg.(sftp.Config))
	case "s3":
		be, err = s3.Open(ctx, cfg.(s3.Config), rt)
	case "gs":
		be, err = gs.Open(cfg.(gs.Config), rt)
	case "azure":
		be, err = azure.Open(cfg.(azure.Config), rt)
	case "swift":
		be, err = swift.Open(ctx, cfg.(swift.Config), rt)
	case "b2":
		be, err = b2.Open(ctx, cfg.(b2.Config), rt)
	case "rest":
		be, err = rest.Open(cfg.(rest.Config), rt)
	case "rclone":
		be, err = rclone.Open(cfg.(rclone.Config), lim)

	default:
		return nil, errors.Fatalf("invalid backend: %q", loc.Scheme)
	}

	if err != nil {
		return nil, errors.Fatalf("unable to open repo at %v: %v", StripPassword(s), err)
	}

	// Install rate limiting on non-HTTP backends.
	if !usesHTTP {
		be = limiter.LimitBackend(be, lim)
	}

	return be, nil
}

func parseConfig(loc location, opts options.Options) (interface{}, error) {
	// only apply options for a particular backend here
	opts = opts.Extract(loc.Scheme)

	switch loc.Scheme {
	case "local":
		cfg := loc.Config.(local.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening local repository at %#v", cfg)
		return cfg, nil

	case "sftp":
		cfg := loc.Config.(sftp.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening sftp repository at %#v", cfg)
		return cfg, nil

	case "s3":
		cfg := loc.Config.(s3.Config)
		if cfg.KeyID == "" {
			cfg.KeyID = os.Getenv("AWS_ACCESS_KEY_ID")
		}

		if cfg.Secret == "" {
			cfg.Secret = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}

		if cfg.KeyID == "" && cfg.Secret != "" {
			return nil, errors.Fatalf("unable to open S3 backend: Key ID ($AWS_ACCESS_KEY_ID) is empty")
		} else if cfg.KeyID != "" && cfg.Secret == "" {
			return nil, errors.Fatalf("unable to open S3 backend: Secret ($AWS_SECRET_ACCESS_KEY) is empty")
		}

		if cfg.Region == "" {
			cfg.Region = os.Getenv("AWS_DEFAULT_REGION")
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening s3 repository at %#v", cfg)
		return cfg, nil

	case "gs":
		cfg := loc.Config.(gs.Config)
		if cfg.ProjectID == "" {
			cfg.ProjectID = os.Getenv("GOOGLE_PROJECT_ID")
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening gs repository at %#v", cfg)
		return cfg, nil

	case "azure":
		cfg := loc.Config.(azure.Config)
		if cfg.AccountName == "" {
			cfg.AccountName = os.Getenv("AZURE_ACCOUNT_NAME")
		}

		if cfg.AccountKey == "" {
			cfg.AccountKey = os.Getenv("AZURE_ACCOUNT_KEY")
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening gs repository at %#v", cfg)
		return cfg, nil

	case "swift":
		cfg := loc.Config.(swift.Config)

		if err := swift.ApplyEnvironment("", &cfg); err != nil {
			return nil, err
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening swift repository at %#v", cfg)
		return cfg, nil

	case "b2":
		cfg := loc.Config.(b2.Config)

		if cfg.AccountID == "" {
			cfg.AccountID = os.Getenv("B2_ACCOUNT_ID")
		}

		if cfg.AccountID == "" {
			return nil, errors.Fatalf("unable to open B2 backend: Account ID ($B2_ACCOUNT_ID) is empty")
		}

		if cfg.Key == "" {
			cfg.Key = os.Getenv("B2_ACCOUNT_KEY")
		}

		if cfg.Key == "" {
			return nil, errors.Fatalf("unable to open B2 backend: Key ($B2_ACCOUNT_KEY) is empty")
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening b2 repository at %#v", cfg)
		return cfg, nil
	case "rest":
		cfg := loc.Config.(rest.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening rest repository at %#v", cfg)
		return cfg, nil
	case "rclone":
		cfg := loc.Config.(rclone.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening rest repository at %#v", cfg)
		return cfg, nil
	}

	return nil, errors.Fatalf("invalid backend: %q", loc.Scheme)
}
