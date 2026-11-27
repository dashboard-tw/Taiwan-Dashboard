package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/config"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/agri"
	"dashboard.tw/backend/internal/sources/alerts"
	"dashboard.tw/backend/internal/sources/animal"
	"dashboard.tw/backend/internal/sources/aqi"
	"dashboard.tw/backend/internal/sources/aviation"
	"dashboard.tw/backend/internal/sources/bike"
	"dashboard.tw/backend/internal/sources/cwa"
	"dashboard.tw/backend/internal/sources/debris"
	"dashboard.tw/backend/internal/sources/disease"
	"dashboard.tw/backend/internal/sources/earthquake"
	"dashboard.tw/backend/internal/sources/economy"
	"dashboard.tw/backend/internal/sources/ev"
	"dashboard.tw/backend/internal/sources/fire"
	"dashboard.tw/backend/internal/sources/flood"
	"dashboard.tw/backend/internal/sources/food"
	"dashboard.tw/backend/internal/sources/fuel"
	"dashboard.tw/backend/internal/sources/housing"
	"dashboard.tw/backend/internal/sources/outage"
	"dashboard.tw/backend/internal/sources/population"
	"dashboard.tw/backend/internal/sources/power"
	"dashboard.tw/backend/internal/sources/rail"
	"dashboard.tw/backend/internal/sources/reservoir"
	"dashboard.tw/backend/internal/sources/resources"
	"dashboard.tw/backend/internal/sources/river"
	"dashboard.tw/backend/internal/sources/school"
	"dashboard.tw/backend/internal/sources/tdx"
	"dashboard.tw/backend/internal/sources/tourism"
	"dashboard.tw/backend/internal/sources/traffic"
	"dashboard.tw/backend/internal/sources/waste"
	"dashboard.tw/backend/internal/sources/weather"
	"dashboard.tw/backend/internal/sources/welfare"
)

type scheduled struct {
	pipeline pipeline.Pipeline
	schedule pipeline.Schedule
}

func registry(cfg config.Config) []scheduled {
	h, m := time.Hour, time.Minute
	reg := []scheduled{
		{&reservoir.RefPipeline{DailyURL: reservoir.DefaultDailyURL, BasicURL: reservoir.DefaultBasicURL},
			pipeline.Schedule{Every: 24 * h, Offset: 10*h + 5*m}}, // daily figures are entered by 09:30
		{&reservoir.ObsPipeline{URL: reservoir.DefaultRealtimeURL},
			pipeline.Schedule{Every: h, Offset: 20 * m}},
		{&river.StationPipeline{URL: river.DefaultStationURL},
			pipeline.Schedule{Every: 24 * h, Offset: 5*h + 20*m}},
		{&river.LevelPipeline{URL: river.DefaultLevelURL},
			pipeline.Schedule{Every: 10 * m, Offset: 7 * m}},
		{&outage.WaterPipeline{Source: "twc", URL: outage.DefaultTWCURL},
			pipeline.Schedule{Every: 30 * m, Offset: 12 * m}},
		{&outage.WaterPipeline{Source: "taipei", URL: outage.DefaultTaipeiURL},
			pipeline.Schedule{Every: 30 * m, Offset: 14 * m}},
		{&outage.PowerPipeline{URL: outage.DefaultPowerURL},
			pipeline.Schedule{Every: 20 * m, Offset: 2 * m}},
		{&earthquake.Pipeline{Kind: "significant", URL: earthquake.DefaultSignificantURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 2 * m}},
		{&earthquake.Pipeline{Kind: "local", URL: earthquake.DefaultLocalURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 2 * m, Offset: m}},
		{&earthquake.TsunamiPipeline{URL: earthquake.DefaultTsunamiURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 2 * m, Offset: m}},
		{&weather.TyphoonPipeline{URL: weather.DefaultTyphoonURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 10 * m, Offset: 3 * m}},
		{&weather.TyphoonWarningPipeline{URL: weather.DefaultTyphoonWarningURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 5 * m, Offset: 4 * m}},
		{&weather.WarningPipeline{URL: weather.DefaultWarningURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 5 * m, Offset: 2 * m}},
		{&weather.RainPipeline{URL: weather.DefaultRainURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 5 * m, Offset: 2 * m}},
		{&weather.ObsPipeline{HourlyURL: weather.DefaultObsHourlyURL, TenMinURL: weather.DefaultObsTenMinURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 5 * m, Offset: 3 * m}},
		{&weather.ForecastPipeline{URL: weather.DefaultForecastURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 15 * m, Offset: 8 * m}},
		{&weather.TownForecastPipeline{URL: weather.DefaultTownForecastURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: h, Offset: 25 * m}},
		{&weather.TownForecastPipeline{Week: true, URL: weather.DefaultTownForecastURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 3 * h, Offset: 40 * m}},
		{&weather.TidePipeline{URL: weather.DefaultTideURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 24 * h, Offset: 3*h + 15*m}},
		{&weather.SunMoonPipeline{SunURL: weather.DefaultSunURL, MoonURL: weather.DefaultMoonURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 24 * h, Offset: 35 * m}},
		{&weather.AdvisoryPipeline{URLs: weather.DefaultAdvisoryURLs, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 10 * m, Offset: 4 * m}},
		{&weather.HealthPipeline{URLs: weather.DefaultHealthURLs, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 3 * h, Offset: 50 * m}},
		{&weather.ClimatePipeline{NormalURL: weather.DefaultNormalURL, RainURL: weather.DefaultRainDailyURL, APIKey: cfg.CwaAPIKey},
			pipeline.Schedule{Every: 24 * h, Offset: 9*h + 5*m}},
		{&alerts.NCDRPipeline{URL: alerts.DefaultFeedURL},
			pipeline.Schedule{Every: 5 * m, Offset: m}},
		{&alerts.ClosurePipeline{URL: alerts.DefaultClosureURL},
			pipeline.Schedule{Every: 10 * m, Offset: 5 * m}},
		{&traffic.SectionPipeline{URL: traffic.DefaultSectionURL, ShapeURL: traffic.DefaultSectionShapeURL},
			pipeline.Schedule{Every: 24 * h, Offset: 4*h + 40*m}},
		{&traffic.LivePipeline{URL: traffic.DefaultLiveURL},
			pipeline.Schedule{Every: 5 * m, Offset: 2 * m}},
		{&fuel.Pipeline{URL: fuel.DefaultHistoryURL},
			pipeline.Schedule{Every: 12 * h, Offset: 7*h + 20*m}},
		{&resources.MedicalPipeline{},
			pipeline.Schedule{Every: 24 * h, Offset: 5*h + 10*m}},
		{&resources.LTCPipeline{URL: resources.DefaultLTCURL, FallbackURL: resources.FallbackLTCURL},
			pipeline.Schedule{Every: 7 * 24 * h, Offset: 5*h + 30*m}},
		{&resources.ShelterPipeline{URL: resources.DefaultShelterURL},
			pipeline.Schedule{Every: 30 * m, Offset: 11 * m}},
		{&resources.ShelterReportPipeline{URL: resources.DefaultShelterReportURL},
			pipeline.Schedule{Every: 10 * m, Offset: 6 * m}},
		{&population.Pipeline{},
			pipeline.Schedule{Every: 7 * 24 * h, Offset: 5*h + 50*m}},
		{&traffic.AccidentPipeline{URL: traffic.DefaultAccidentURL},
			pipeline.Schedule{Every: 24 * h, Offset: 6*h + 10*m}},
		{&tourism.AttractionPipeline{URL: tourism.DefaultAttractionURL},
			pipeline.Schedule{Every: 24 * h, Offset: 6*h + 30*m}},
		{&tourism.HotelPipeline{URL: tourism.DefaultHotelURL},
			pipeline.Schedule{Every: 24 * h, Offset: 6*h + 40*m}},
		{&river.QualityPipeline{URL: river.DefaultQualityURL, APIKey: cfg.MoenvAPIKey},
			pipeline.Schedule{Every: 24 * h, Offset: 6*h + 50*m}},
		{&power.RenewablePipeline{URL: power.DefaultRenewableURL},
			pipeline.Schedule{Every: 24 * h, Offset: 7*h + 5*m}},
		{&school.Pipeline{},
			pipeline.Schedule{Every: 7 * 24 * h, Offset: 7*h + 35*m}},
		{&population.VitalPipeline{},
			pipeline.Schedule{Every: 24 * h, Offset: 7*h + 45*m}},
		{&welfare.ChildcarePipeline{URL: welfare.DefaultURL},
			pipeline.Schedule{Every: 7 * 24 * h, Offset: 7*h + 55*m}},
		{&power.SalesPipeline{URL: power.DefaultSalesURL},
			pipeline.Schedule{Every: 24 * h, Offset: 8*h + 5*m}},
		{&waste.Pipeline{URL: waste.DefaultURL, APIKey: cfg.MoenvAPIKey},
			pipeline.Schedule{Every: 24 * h, Offset: 8*h + 15*m}},
		{&agri.Pipeline{URL: agri.DefaultURL},
			pipeline.Schedule{Every: 8 * h, Offset: 2*h + 25*m}},
		{&food.Pipeline{URL: food.DefaultURL},
			pipeline.Schedule{Every: 24 * h, Offset: 8*h + 25*m}},
		{&fire.Pipeline{},
			pipeline.Schedule{Every: 7 * 24 * h, Offset: 8*h + 35*m}},
		{&housing.Pipeline{},
			pipeline.Schedule{Every: 24 * h, Offset: 3*h + 15*m}},
		{&fuel.StationPipeline{URL: fuel.DefaultStationURL},
			pipeline.Schedule{Every: 24 * h, Offset: 8*h + 45*m}},
		{&disease.DenguePipeline{URL: disease.DefaultDengueURL},
			pipeline.Schedule{Every: 6 * h, Offset: 50 * m}},
		{&disease.VisitPipeline{URLs: disease.DefaultVisitURLs},
			pipeline.Schedule{Every: 24 * h, Offset: 9*h + 5*m}},
		{&economy.IndicatorPipeline{CPIURL: economy.DefaultCPIURL, UnemploymentURL: economy.DefaultUnemploymentURL,
			CycleURL: economy.DefaultCycleURL},
			pipeline.Schedule{Every: 24 * h, Offset: 9*h + 15*m}},
		{&economy.IncomePipeline{URL: economy.DefaultIncomeURL},
			pipeline.Schedule{Every: 7 * 24 * h, Offset: 9*h + 25*m}},
		{&animal.AdoptPipeline{URL: animal.DefaultAdoptURL},
			pipeline.Schedule{Every: 6 * h, Offset: 1*h + 35*m}},
		{&animal.StatsPipeline{URL: animal.DefaultStatsURL},
			pipeline.Schedule{Every: 24 * h, Offset: 9*h + 35*m}},
		{&debris.Pipeline{URL: debris.DefaultURL},
			pipeline.Schedule{Every: 10 * m, Offset: 4 * m}},
		{&flood.Pipeline{URL: flood.DefaultURL},
			pipeline.Schedule{Every: 10 * m, Offset: 6 * m}},
		{&agri.PoultryPipeline{URL: agri.DefaultPoultryURL},
			pipeline.Schedule{Every: 24 * h, Offset: 9*h + 45*m}},
		{&agri.HogPipeline{URL: agri.DefaultHogURL},
			pipeline.Schedule{Every: 24 * h, Offset: 9*h + 55*m}},
		{&aqi.Pipeline{URL: aqi.DefaultURL, APIKey: cfg.MoenvAPIKey},
			pipeline.Schedule{Every: h, Offset: 25 * m}},
		{&aqi.UVPipeline{URL: aqi.DefaultUVURL, APIKey: cfg.MoenvAPIKey},
			pipeline.Schedule{Every: h, Offset: 15 * m}},
		{&aqi.ForecastPipeline{URL: aqi.DefaultForecastURL, APIKey: cfg.MoenvAPIKey},
			pipeline.Schedule{Every: 30 * m, Offset: 5 * m}},
		{&power.GenPipeline{URL: power.DefaultGenURL},
			pipeline.Schedule{Every: 10 * m, Offset: 3 * m}},
		{&power.RegionPipeline{URL: power.DefaultRegionURL},
			pipeline.Schedule{Every: 10 * m, Offset: 4 * m}},
		{&power.LoadPipeline{URL: power.DefaultLoadURL},
			pipeline.Schedule{Every: 10 * m, Offset: 4 * m}},
		{&power.PeakPipeline{URL: power.DefaultLoadURL},
			pipeline.Schedule{Every: 12 * h, Offset: 8*h + 30*m}},
		{&power.DailyPipeline{URL: power.DefaultDailyURL},
			pipeline.Schedule{Every: 24 * h, Offset: 6 * h}},
		{&power.ForecastPipeline{URL: power.DefaultForecastURL},
			pipeline.Schedule{Every: 24 * h, Offset: 6*h + 10*m}},
	}
	if cfg.ImageDir != "" {
		for i, id := range []string{"radar", "satellite"} {
			prod := weather.ImageProducts[id]
			reg = append(reg, scheduled{
				&weather.ImagePipeline{Product: prod, URL: cwa.FileAPI + prod.DataID, APIKey: cfg.CwaAPIKey, Dir: cfg.ImageDir},
				pipeline.Schedule{Every: 5 * m, Offset: time.Duration(i+1) * m},
			})
		}
		reg = append(reg, scheduled{
			&weather.RadarLayerPipeline{URL: weather.DefaultRadarGridURL, APIKey: cfg.CwaAPIKey, Dir: cfg.ImageDir},
			pipeline.Schedule{Every: 5 * m, Offset: 3 * m},
		})
		reg = append(reg, scheduled{
			&weather.CloudPipeline{TileURL: weather.DefaultCloudTileURL, ColorURL: weather.DefaultCloudColorURL, Dir: cfg.ImageDir},
			pipeline.Schedule{Every: 10 * m, Offset: 4 * m},
		})
		reg = append(reg, scheduled{
			&weather.WindPipeline{URL: weather.DefaultWindURL, Dir: cfg.ImageDir},
			pipeline.Schedule{Every: 30 * m, Offset: 9 * m},
		})
	}
	accounts := tdxAccounts(cfg)
	rail1, bike2, flight3, ev3 := accounts[0], accounts[0], accounts[0], accounts[0]
	switch len(accounts) {
	case 2:
		bike2, ev3 = accounts[1], accounts[1]
	case 3:
		bike2, flight3, ev3 = accounts[1], accounts[2], accounts[2]
	}
	if len(accounts) > 1 {
		slog.Info("TDX: splitting the pipelines across accounts", "accounts", len(accounts))
	}
	t := rail1
	reg = append(reg,
		scheduled{&rail.StationPipeline{TDX: t}, pipeline.Schedule{Every: 7 * 24 * h, Offset: 4*h + 50*m}},
		scheduled{&bike.StationPipeline{TDX: bike2}, pipeline.Schedule{Every: 7 * 24 * h, Offset: 4*h + 20*m}},
		scheduled{&aviation.RefPipeline{TDX: flight3}, pipeline.Schedule{Every: 7 * 24 * h, Offset: 4*h + 35*m}},
		scheduled{&ev.StationPipeline{TDX: ev3}, pipeline.Schedule{Every: 7 * 24 * h, Offset: 5*h + 5*m}},
		scheduled{&rail.AlertPipeline{TDX: t}, pipeline.Schedule{Every: 2 * h, Offset: 7 * m}},
		scheduled{&aviation.FlightPipeline{TDX: flight3}, pipeline.Schedule{Every: 3 * h, Offset: 2 * m}},
		scheduled{&rail.LivePipeline{TDX: t}, pipeline.Schedule{Every: h, Offset: 3 * m}},
		scheduled{&bike.AvailabilityPipeline{TDX: bike2}, pipeline.Schedule{Every: 12 * h, Offset: 8*h + 12*m}},
		scheduled{&ev.LivePipeline{TDX: ev3}, pipeline.Schedule{Every: 24 * h, Offset: 12*h + 22*m}},
	)
	return reg
}

func tdxAccounts(cfg config.Config) []*tdx.Client {
	accounts := []*tdx.Client{tdx.NewClient(cfg.TdxClientID, cfg.TdxClientSecret)}
	for _, k := range [][2]string{{cfg.TdxClientID2, cfg.TdxClientSecret2}, {cfg.TdxClientID3, cfg.TdxClientSecret3}} {
		if k[0] != "" && k[1] != "" {
			accounts = append(accounts, tdx.NewClient(k[0], k[1]))
		}
	}
	return accounts
}

func pipelinesOf(reg []scheduled) []pipeline.Pipeline {
	out := make([]pipeline.Pipeline, len(reg))
	for i, s := range reg {
		out[i] = s.pipeline
	}
	return out
}

func newWorkerCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Run the data pipelines on their schedules",
		Long: "Run every data pipeline once at start-up, then on its schedule, until stopped with Ctrl+C.\n" +
			"Use \"worker run <dataset>\" to run pipelines once, and \"worker list\" to see them.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RefuseShared("worker"); err != nil {
				return err
			}
			runner, closeDB, err := newRunner(cmd.Context(), version)
			if err != nil {
				return err
			}
			defer closeDB()

			reg := registry(cfg)
			if err := runner.RegisterDatasets(cmd.Context(), pipelinesOf(reg)); err != nil {
				return err
			}
			if cfg.MoenvAPIKey == "" {
				slog.Warn("MOENV_API_KEY is not set; the aqi_obs pipeline will report an error until it is")
			}
			if cfg.TdxClientID == "" || cfg.TdxClientSecret == "" {
				slog.Warn("TDX_CLIENT_ID or TDX_CLIENT_SECRET is not set; the rail and YouBike pipelines will report an error until they are")
			}
			if cfg.CwaAPIKey == "" {
				slog.Warn("CWA_API_KEY is not set; the earthquake and weather pipelines will report an error until it is")
			}

			var jobs []pipeline.Job
			for _, s := range reg {
				jobs = append(jobs, pipeline.PipelineJob(runner, s.pipeline, s.schedule))
			}
			jobs = append(jobs, maintenanceJobs(runner)...)
			if cfg.DownloadDir != "" {
				jobs = append(jobs, exportJob(runner, cfg.DownloadDir))
			} else {
				slog.Warn("DOWNLOAD_DIR is not set; monthly bulk files are not written")
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			slog.Info("worker started", "pipelines", len(reg))
			(&pipeline.Worker{Jobs: jobs, Now: time.Now, Logger: slog.Default()}).Start(ctx)
			slog.Info("worker stopped")
			return nil
		},
	}
	cmd.AddCommand(newWorkerRunCmd(version), newWorkerListCmd())
	return cmd
}

func exportJob(r *pipeline.Runner, dir string) pipeline.Job {
	e := &opendata.Exporter{Service: &opendata.Service{DB: r.DB}, Dir: dir, Now: time.Now, Logger: slog.Default()}
	return pipeline.Job{
		Name:     "export_downloads",
		Schedule: pipeline.Schedule{Every: 24 * time.Hour, Offset: 3*time.Hour + 30*time.Minute},
		Run: func(ctx context.Context) error {
			res, err := e.Run(ctx)
			if res.Written > 0 {
				slog.Info("bulk files updated", "written", res.Written, "current", res.Current)
			}
			return err
		},
	}
}

func maintenanceJobs(r *pipeline.Runner) []pipeline.Job {
	return []pipeline.Job{
		{
			Name:     "mark_stale",
			Schedule: pipeline.Schedule{Every: 5 * time.Minute},
			Run: func(ctx context.Context) error {
				n, err := pipeline.MarkStale(ctx, r.DB)
				if n > 0 {
					slog.Warn("datasets marked stale", "count", n)
				}
				if err != nil {
					return err
				}
				released, err := pipeline.ReleaseOrphanRuns(ctx, r.DB)
				if released > 0 {
					slog.Warn("interrupted pipeline runs marked failed", "count", released)
				}
				return err
			},
		},
		{
			Name:     "purge_personal_data",
			Schedule: pipeline.Schedule{Every: 24 * time.Hour, Offset: 4 * time.Hour},
			Run: func(ctx context.Context) error {
				p, err := auth.Purge(ctx, r.DB, time.Now())
				if p.Sessions+p.AuditIPs+p.OAuthLogins+p.Audit+p.Runs+p.Clients > 0 {
					slog.Info("old records purged", "sessions", p.Sessions, "audit_ips", p.AuditIPs,
						"oauth_logins", p.OAuthLogins, "audit", p.Audit, "pipeline_runs", p.Runs,
						"deleted_clients", p.Clients)
				}
				return err
			},
		},
	}
}

func newWorkerRunCmd(version string) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "run [dataset...]",
		Short: "Run pipelines once and exit",
		Example: "  dashboardtw worker run power_gen\n" +
			"  dashboardtw worker run --all",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RefuseShared("worker run"); err != nil {
				return err
			}
			reg := registry(cfg)
			byID := map[string]pipeline.Pipeline{}
			for _, s := range reg {
				byID[s.pipeline.Dataset().ID] = s.pipeline
			}

			var selected []pipeline.Pipeline
			switch {
			case all:
				selected = pipelinesOf(reg)
			case len(args) == 0:
				return fmt.Errorf("name at least one dataset, or use --all (see \"worker list\")")
			default:
				for _, id := range args {
					p, ok := byID[id]
					if !ok {
						return fmt.Errorf("unknown dataset %q (see \"worker list\")", id)
					}
					selected = append(selected, p)
				}
			}

			runner, closeDB, err := newRunner(cmd.Context(), version)
			if err != nil {
				return err
			}
			defer closeDB()
			if err := runner.RegisterDatasets(cmd.Context(), pipelinesOf(reg)); err != nil {
				return err
			}

			failed := 0
			for _, p := range selected {
				res, err := runner.Run(cmd.Context(), p)
				id := p.Dataset().ID
				if err != nil {
					failed++
					fmt.Fprintf(cmd.OutOrStdout(), "%-24s failed: %v\n", id, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s fetched %d, inserted %d, warnings %d\n",
					id, res.RowsFetched, res.RowsInserted, len(res.Warnings))
				for _, w := range res.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "    ! %s\n", w)
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d pipelines failed", failed, len(selected))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "run every pipeline")
	return cmd
}

func newWorkerListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the pipelines and their schedules",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			for _, s := range registry(cfg) {
				d := s.pipeline.Dataset()
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s every %-8s +%-8s %s（%s）\n",
					d.ID, s.schedule.Every, s.schedule.Offset, d.Name, d.SourceAgency)
			}
			return nil
		},
	}
}

func newRunner(ctx context.Context, version string) (*pipeline.Runner, func(), error) {
	sqlDB, gdb, err := openDatabase(ctx)
	if err != nil {
		return nil, nil, err
	}
	r := &pipeline.Runner{
		DB:      sqlDB,
		Fetch:   pipeline.NewFetcher(version),
		Now:     time.Now,
		Logger:  slog.Default(),
		Timeout: 5 * time.Minute,
	}
	r.OnFinish = pipelineNotifier(sqlDB)
	return r, func() { _ = db.Close(gdb) }, nil
}
