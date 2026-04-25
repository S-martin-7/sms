package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/sms"
)

func runInbound(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl inbound assign|list|unassign")
	}
	sub, rest := args[0], args[1:]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	svc := sms.NewService(pool)

	switch sub {
	case "assign":
		fs := flag.NewFlagSet("inbound assign", flag.ExitOnError)
		msisdn := fs.String("msisdn", "", "destination MSISDN (E.164 without leading +)")
		tenantID := fs.Int64("tenant-id", 0, "tenant id that owns this number")
		label := fs.String("label", "", "optional label, e.g. 'shortcode marketing'")
		_ = fs.Parse(rest)
		if *msisdn == "" || *tenantID == 0 {
			return fmt.Errorf("--msisdn and --tenant-id required")
		}
		row, err := svc.AssignInboundNumber(ctx, *msisdn, *tenantID, *label)
		if err != nil {
			return err
		}
		fmt.Printf("inbound number assigned: msisdn=%s tenant_id=%d label=%q\n",
			row.MSISDN, row.TenantID, row.Label)
		return nil
	case "list":
		rows, err := svc.ListInboundNumbers(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Println("(no inbound numbers configured)")
			return nil
		}
		for _, r := range rows {
			fmt.Printf("%-20s\ttenant=%d\tlabel=%q\tcreated=%s\n",
				r.MSISDN, r.TenantID, r.Label, r.CreatedAt.Format(time.RFC3339))
		}
		return nil
	case "unassign":
		fs := flag.NewFlagSet("inbound unassign", flag.ExitOnError)
		msisdn := fs.String("msisdn", "", "MSISDN to remove")
		_ = fs.Parse(rest)
		if *msisdn == "" {
			return fmt.Errorf("--msisdn required")
		}
		if err := svc.UnassignInboundNumber(ctx, *msisdn); err != nil {
			return err
		}
		fmt.Printf("inbound number removed: msisdn=%s\n", *msisdn)
		return nil
	default:
		return fmt.Errorf("unknown inbound subcommand: %s", sub)
	}
}
