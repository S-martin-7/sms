package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// runSeed populates a tenant with realistic-looking sample data so the
// dashboard's flows can be reviewed without manually creating everything.
//
// Idempotent: re-running upserts contacts and skips lists that already
// exist by name. Messages are always added (no idempotency key) so the
// counts grow if you run it multiple times.
func runSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	tenantID := fs.Int64("tenant-id", 0, "tenant id to seed")
	contactCount := fs.Int("contacts", 50, "how many contacts to create")
	messageCount := fs.Int("messages", 60, "how many historical messages to create")
	scheduledCount := fs.Int("scheduled", 5, "how many scheduled sends to create")
	_ = fs.Parse(args)
	if *tenantID == 0 {
		return fmt.Errorf("--tenant-id required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	q := sqlcgen.New(pool)

	// Sanity check tenant exists.
	if _, err := q.GetTenantByID(ctx, *tenantID); err != nil {
		return fmt.Errorf("tenant %d not found: %w", *tenantID, err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 1) Lists — create three fixed groups; idempotent via ON CONFLICT not
	//    available here, so we just ignore "already exists".
	listNames := []struct {
		name        string
		description string
		share       float64 // approx share of contacts
	}{
		{"Colegio", "Apoderados del establecimiento", 0.35},
		{"Gym", "Miembros activos del gimnasio", 0.45},
		{"Empresa", "Personal de la empresa", 0.20},
	}
	listIDs := map[string]int64{}
	for _, l := range listNames {
		row, err := q.CreateContactList(ctx, sqlcgen.CreateContactListParams{
			TenantID: *tenantID, Name: l.name, Description: ptrStr(l.description),
		})
		if err != nil {
			// Probably duplicate — fetch the existing id by name.
			var id int64
			err2 := pool.QueryRow(ctx,
				`SELECT id FROM contact_lists WHERE tenant_id = $1 AND name = $2`,
				*tenantID, l.name).Scan(&id)
			if err2 != nil {
				return fmt.Errorf("seed list %q: %w", l.name, err)
			}
			listIDs[l.name] = id
			fmt.Printf("  · lista existente: %s (id=%d)\n", l.name, id)
		} else {
			listIDs[l.name] = row.ID
			fmt.Printf("  + lista nueva: %s (id=%d)\n", l.name, row.ID)
		}
	}

	// 2) Contacts — Chilean-flavoured fake names + 569XXXXXXXX msisdns.
	//    Upsert por (tenant, msisdn) para que re-correr no falle.
	first := []string{"Pablo","Lucia","Mateo","Sofía","Diego","Catalina","Joaquín","Valentina","Tomás","Florencia","Benjamín","Antonia","Maximiliano","Isidora","Vicente","Camila","Agustín","Trinidad","Bruno","Constanza","Felipe","Renata","Cristóbal","Magdalena","Ignacio","Amparo"}
	last := []string{"González","Muñoz","Rojas","Díaz","Pérez","Soto","Contreras","Silva","Martínez","Sepúlveda","Morales","Torres","Castillo","Reyes","Gutiérrez","Vásquez","Tapia","Vargas","Flores","Cortés"}

	created := 0
	for i := 0; i < *contactCount; i++ {
		name := first[rng.Intn(len(first))] + " " + last[rng.Intn(len(last))]
		// 569 + 8 dígitos
		msisdn := fmt.Sprintf("569%08d", 10000000+rng.Intn(89999999))
		notes := ""
		if rng.Float64() < 0.2 {
			notes = []string{"Cliente VIP", "Pago al día", "Solo WhatsApp", "No molestar después de 20:00", "Cuenta corporativa"}[rng.Intn(5)]
		}
		row, err := q.UpsertContact(ctx, sqlcgen.UpsertContactParams{
			TenantID: *tenantID, Msisdn: msisdn, Name: ptrStr(name), Notes: ptrStr(notes), Column5: []byte(`{}`),
		})
		if err != nil {
			fmt.Printf("  ! contacto %s: %v\n", msisdn, err)
			continue
		}
		// Asignar a una lista según pesos.
		r := rng.Float64()
		var pickedList string
		switch {
		case r < listNames[0].share:
			pickedList = "Colegio"
		case r < listNames[0].share+listNames[1].share:
			pickedList = "Gym"
		default:
			pickedList = "Empresa"
		}
		_ = q.AddContactsToList(ctx, sqlcgen.AddContactsToListParams{
			Column1:    listIDs[pickedList],
			TenantID:   *tenantID,
			ContactIds: []int64{row.ID},
		})
		// 5% opt-out
		if rng.Float64() < 0.05 {
			_ = q.SetContactOptOut(ctx, sqlcgen.SetContactOptOutParams{
				ID: row.ID, TenantID: *tenantID, OptOut: true,
			})
		}
		created++
	}
	fmt.Printf("  ✓ %d contactos sembrados\n", created)

	// 3) Historical messages — variados estados + fechas en últimos 30 días.
	statuses := []struct {
		status   string
		weight   float64 // probabilidad acumulada
		hasFinal bool
	}{
		{"delivered", 0.70, true},
		{"sent", 0.80, false},  // todavía sin DLR
		{"undelivered", 0.88, true},
		{"rejected", 0.95, true},
		{"failed", 0.98, true},
		{"queued", 1.00, false},
	}
	senders := []string{"MiMarca", "Recordatorio", "Promo", "Aviso", "OTP"}
	texts := []string{
		"Hola, recordamos su cita para mañana a las 10:00. Responda CONFIRMA o CANCELA.",
		"Su pedido #42 fue despachado. Llega entre 14:00 y 18:00 hoy.",
		"OTP de verificación: 482910. No lo comparta.",
		"Promoción: 30% de descuento solo hoy con el código LUNES30.",
		"Su pago de $24.990 fue recibido. Gracias por preferirnos.",
		"Reunión de apoderados este jueves 18:30 en el salón principal.",
	}
	msgs := 0
	for i := 0; i < *messageCount; i++ {
		status := pickWeighted(rng.Float64(), statuses)
		recipient := fmt.Sprintf("569%08d", 10000000+rng.Intn(89999999))
		sender := senders[rng.Intn(len(senders))]
		text := texts[rng.Intn(len(texts))]
		// created_at: en últimos 30 días, escalonado.
		createdAt := time.Now().Add(-time.Duration(rng.Intn(30*24)) * time.Hour).
			Add(-time.Duration(rng.Intn(60)) * time.Minute)
		id := uuid.New()
		// Insertamos directo via SQL para controlar created_at/sent_at/final_at.
		hMsg := fmt.Sprintf("horisen-fake-%d", rng.Int63())
		var sentAt, finalAt *time.Time
		var errCode, errMsg *string
		if status.status != "queued" {
			t := createdAt.Add(time.Duration(1+rng.Intn(5)) * time.Second)
			sentAt = &t
		}
		if status.hasFinal {
			t := createdAt.Add(time.Duration(2+rng.Intn(20)) * time.Second)
			finalAt = &t
		}
		if status.status == "rejected" || status.status == "failed" {
			c := []string{"103", "104", "108", "112"}[rng.Intn(4)]
			m := []string{"Invalid receiver", "Sending from client's IP not allowed", "Receiver blocked", "Operator unreachable"}[rng.Intn(4)]
			errCode, errMsg = &c, &m
		} else if status.status == "undelivered" {
			c := "30"
			m := "MS busy for MT SMS"
			errCode, errMsg = &c, &m
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO messages (id, tenant_id, sender, recipient, text, dcs, num_parts, status,
			                     horisen_msg_id, error_code, error_message, attempts,
			                     created_at, sent_at, final_at)
			VALUES ($1,$2,$3,$4,$5,'GSM',1,$6,$7,$8,$9,$10,$11,$12,$13)`,
			pgtype.UUID{Bytes: id, Valid: true}, *tenantID, sender, recipient, text, status.status,
			ptrStr(hMsg), errCode, errMsg, 1+rng.Intn(2),
			pgtype.Timestamptz{Time: createdAt, Valid: true},
			tsOrNull(sentAt), tsOrNull(finalAt),
		)
		if err == nil {
			msgs++
		}
	}
	fmt.Printf("  ✓ %d mensajes históricos sembrados\n", msgs)

	// 4) Scheduled sends — mezcla de one-shot futuros + recurrentes.
	created = 0
	for i := 0; i < *scheduledCount; i++ {
		isWeekly := i%2 == 0
		when := time.Now().Add(time.Duration(2+rng.Intn(48)) * time.Hour)
		// Recipients: o bien una lista al azar, o 5-15 números pegados.
		var listID *int64
		var recips []byte
		var name string
		if rng.Float64() < 0.5 {
			lname := []string{"Colegio", "Gym", "Empresa"}[rng.Intn(3)]
			id := listIDs[lname]
			listID = &id
			name = fmt.Sprintf("%s · campaña #%d", lname, i+1)
		} else {
			n := 5 + rng.Intn(10)
			arr := make([]string, n)
			for j := range arr {
				arr[j] = fmt.Sprintf("569%08d", 10000000+rng.Intn(89999999))
			}
			recips, _ = json.Marshal(arr)
			name = fmt.Sprintf("Lote ad-hoc #%d", i+1)
		}
		var rec *string
		var days []int16
		if isWeekly {
			r := "weekly"
			rec = &r
			// Subset aleatorio de días: lun/mié/vie o todos los días hábiles
			if rng.Float64() < 0.5 {
				days = []int16{1, 3, 5}
			} else {
				days = []int16{1, 2, 3, 4, 5}
			}
		}
		_, err := q.CreateScheduledSend(ctx, sqlcgen.CreateScheduledSendParams{
			TenantID:       *tenantID,
			Name:           ptrStr(name),
			Sender:         senders[rng.Intn(len(senders))],
			Text:           texts[rng.Intn(len(texts))],
			Recipients:     recips,
			ListID:         listID,
			SendAt:         pgtype.Timestamptz{Time: when, Valid: true},
			Recurrence:     rec,
			RecurrenceDays: days,
			Column10:       "America/Santiago",
		})
		if err == nil {
			created++
		}
	}
	fmt.Printf("  ✓ %d envíos programados sembrados\n", created)

	fmt.Println("Listo.")
	return nil
}

// helpers ----------------------------------------------------------------

func ptrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func tsOrNull(t *time.Time) any {
	if t == nil {
		return nil
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func pickWeighted(r float64, items []struct {
	status   string
	weight   float64
	hasFinal bool
}) struct {
	status   string
	weight   float64
	hasFinal bool
} {
	for _, it := range items {
		if r <= it.weight {
			return it
		}
	}
	return items[len(items)-1]
}

// silence unused if helpers move
var _ = strings.TrimSpace
