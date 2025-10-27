/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alecthomas/kingpin/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"

	"github.com/rossigee/provider-backblaze/apis"
	backblazecontroller "github.com/rossigee/provider-backblaze/internal/controller"
	"github.com/rossigee/provider-backblaze/internal/features"
	"github.com/rossigee/provider-backblaze/internal/version"
)

func main() {
	var (
		app              = kingpin.New(filepath.Base(os.Args[0]), "Backblaze support for Crossplane.").DefaultEnvars()
		debug            = app.Flag("debug", "Run with debug logging.").Short('d').Bool()
		syncInterval     = app.Flag("sync", "Sync interval controls how often all resources will be double checked for drift.").Short('s').Default("1h").Duration()
		pollInterval     = app.Flag("poll", "Poll interval controls how often an individual resource should be checked for drift.").Default("1m").Duration()
		leaderElection   = app.Flag("leader-election", "Use leader election for the controller manager.").Short('l').Default("false").Bool()
		maxReconcileRate = app.Flag("max-reconcile-rate", "The global maximum rate per second at which resources may checked for drift from the desired state.").Default("10").Int()

		_                          = app.Flag("namespace", "Namespace used to set as default scope in default secret store config.").Default("crossplane-system").Envar("POD_NAMESPACE").String()
		enableExternalSecretStores = app.Flag("enable-external-secret-stores", "Enable support for ExternalSecretStores.").Default("false").Bool()
		enableManagementPolicies   = app.Flag("enable-management-policies", "Enable support for Management Policies.").Default("true").Bool()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	zl := zap.New(zap.UseDevMode(*debug))
	log := logging.NewLogrLogger(zl.WithName("provider-backblaze"))

	// Always set the controller-runtime logger to prevent logging errors
	// Use info level for non-debug mode to reduce verbosity
	if *debug {
		ctrl.SetLogger(zl)
	} else {
		// Set logger with higher level to reduce verbosity in production
		ctrl.SetLogger(zl.V(1))
	}

	// currently, we configure the jitter to be the 5% of the poll interval
	pollJitter := time.Duration(float64(*pollInterval) * 0.05)
	// Log startup information with build and configuration details
	log.Info("Provider starting up",
		"provider", "provider-backblaze",
		"version", version.Version,
		"go-version", runtime.Version(),
		"platform", runtime.GOOS+"/"+runtime.GOARCH,
		"sync-interval", syncInterval.String(),
		"poll-interval", pollInterval.String(),
		"poll-jitter", pollJitter,
		"max-reconcile-rate", *maxReconcileRate,
		"leader-election", *leaderElection,
		"debug-mode", *debug)

	log.Debug("Detailed startup configuration",
		"sync-interval", syncInterval.String(),
		"poll-interval", pollInterval.String(),
		"poll-jitter", pollJitter,
		"max-reconcile-rate", *maxReconcileRate)

	cfg, err := ctrl.GetConfig()
	kingpin.FatalIfError(err, "Cannot get API server rest config")

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		LeaderElection:   *leaderElection,
		LeaderElectionID: "crossplane-leader-election-provider-backblaze",
		Cache: cache.Options{
			SyncPeriod: syncInterval,
		},
		// controller-runtime uses both ConfigMaps and Leases for leader
		// election by default. Leases expire after 15 seconds, with a
		// 10 second renewal deadline. We've observed leader loss due to
		// renewal deadlines being exceeded when under high load - i.e.
		// hundreds of reconciles per second and ~200rps to the API
		// server. Switching to Leases only and longer leases appears to
		// alleviate this.
		LeaderElectionResourceLock: "leases",
		LeaseDuration:              func() *time.Duration { d := 60 * time.Second; return &d }(),
		RenewDeadline:              func() *time.Duration { d := 50 * time.Second; return &d }(),
	})
	kingpin.FatalIfError(err, "Cannot create controller manager")

	// Initialize feature flags
	featureFlags := &feature.Flags{}
	if *enableExternalSecretStores {
		featureFlags.Enable(features.EnableAlphaExternalSecretStores)
		log.Info("Alpha feature enabled", "flag", features.EnableAlphaExternalSecretStores)
	}
	if *enableManagementPolicies {
		featureFlags.Enable(features.EnableAlphaManagementPolicies)
		log.Info("Alpha feature enabled", "flag", features.EnableAlphaManagementPolicies)
	}

	o := controller.Options{
		Logger:                  log,
		MaxConcurrentReconciles: *maxReconcileRate,
		PollInterval:            *pollInterval,
		GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
		Features:                featureFlags,
	}

	kingpin.FatalIfError(apis.AddToScheme(mgr.GetScheme()), "Cannot add Backblaze APIs to scheme")
	kingpin.FatalIfError(backblazecontroller.Setup(mgr, o), "Cannot setup controllers")

	kingpin.FatalIfError(mgr.AddHealthzCheck("healthz", healthz.Ping), "Cannot add health check")
	kingpin.FatalIfError(mgr.AddReadyzCheck("readyz", healthz.Ping), "Cannot add ready check")

	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "Cannot start controller manager")
}
