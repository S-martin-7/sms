<?php
/**
 * Cliente PHP para la pasarela SMS — caso facturación.
 *
 * Uso:
 *   SMS_API_KEY="sk_live_..." php sms_cliente.php
 *
 * Requiere: ext-curl, ext-json (estándar en cualquier instalación).
 */

declare(strict_types=1);

const BASE_URL = 'https://sms.aipanel.cl';
const TIMEOUT  = 10;
const MAX_RETRIES = 3;

$apiKey = getenv('SMS_API_KEY') ?: die("Falta SMS_API_KEY en el entorno\n");

class SMSException extends RuntimeException {
    public string $code;
    public string $requestId;
    public function __construct(string $code, string $message, string $requestId = '') {
        parent::__construct("[$code] $message" . ($requestId ? " (req=$requestId)" : ''));
        $this->code = $code;
        $this->requestId = $requestId;
    }
}

/**
 * Envía un SMS. Reintenta en 429 (rate_limited) respetando Retry-After.
 *
 * @param array{sender:string,to:string,text:string,client_ref?:string} $payload
 * @return array  Cuerpo del 202 (o {status:'duplicate', ...} si fue idempotente).
 * @throws SMSException
 */
function enviarSMS(string $apiKey, array $payload): array {
    for ($attempt = 1; $attempt <= MAX_RETRIES; $attempt++) {
        $ch = curl_init(BASE_URL . '/v1/sms');
        curl_setopt_array($ch, [
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_HEADER => true,
            CURLOPT_POST => true,
            CURLOPT_POSTFIELDS => json_encode($payload, JSON_UNESCAPED_UNICODE),
            CURLOPT_HTTPHEADER => [
                "X-API-Key: $apiKey",
                'Content-Type: application/json',
            ],
            CURLOPT_TIMEOUT => TIMEOUT,
        ]);
        $raw = curl_exec($ch);
        if ($raw === false) {
            $err = curl_error($ch);
            curl_close($ch);
            throw new SMSException('network_error', $err);
        }
        $status = curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
        $headerSize = curl_getinfo($ch, CURLINFO_HEADER_SIZE);
        $headers = parseHeaders(substr($raw, 0, $headerSize));
        $body = json_decode(substr($raw, $headerSize), true) ?? [];
        curl_close($ch);

        $requestId = $headers['x-request-id'] ?? '';
        if (isset($headers['x-daily-quota-used'])) {
            error_log("quota {$headers['x-daily-quota-used']}/{$headers['x-daily-quota-limit']}");
        }

        if ($status === 202) return $body;

        if ($status === 409) {
            return [
                'status' => 'duplicate',
                'client_ref' => $payload['client_ref'] ?? null,
                'id' => null,
            ];
        }

        if ($status === 429) {
            $code = $body['error']['code'] ?? '';
            if ($code === 'daily_quota_exceeded') {
                throw new SMSException('daily_quota_exceeded',
                    $body['error']['message'] ?? '', $requestId);
            }
            $wait = min((int)($headers['retry-after'] ?? '5'), 60);
            error_log("rate_limited; sleeping {$wait}s (attempt $attempt/" . MAX_RETRIES . ")");
            sleep($wait);
            continue;
        }

        // No recuperable
        throw new SMSException(
            $body['error']['code'] ?? 'unknown',
            $body['error']['message'] ?? '',
            $requestId
        );
    }
    throw new SMSException('max_retries_exceeded', 'failed after ' . MAX_RETRIES . ' attempts');
}

function parseHeaders(string $raw): array {
    $out = [];
    foreach (explode("\r\n", trim($raw)) as $line) {
        if (str_contains($line, ':')) {
            [$k, $v] = explode(':', $line, 2);
            $out[strtolower(trim($k))] = trim($v);
        }
    }
    return $out;
}

// ──────────────────────────────────────────────────────────────────────
// Caso de uso real: facturación
// ──────────────────────────────────────────────────────────────────────

function notificarFactura(string $apiKey, array $args): array {
    $monto = number_format($args['monto'], 0, ',', '.');
    $texto = sprintf(
        'Estimado %s: su factura %s emitida el %s por $%s vence en %d días.',
        $args['nombre'], $args['factura'], $args['fecha'], $monto, $args['dias']
    );
    return enviarSMS($apiKey, [
        'sender'     => 'Segtelco',
        'to'         => $args['celular'],
        'text'       => $texto,
        'client_ref' => sprintf('factura-%s-r%d', $args['factura'], $args['recordatorio'] ?? 1),
    ]);
}

// ──────────────────────────────────────────────────────────────────────
// Entry point
// ──────────────────────────────────────────────────────────────────────

try {
    $res = notificarFactura($apiKey, [
        'nombre'       => 'Rodrigo Quiroga',
        'celular'      => '+56985917376',
        'factura'      => '1234',
        'fecha'        => '22-01-2026',
        'monto'        => 200000,
        'dias'         => 5,
        'recordatorio' => 1,
    ]);
    echo "Enviado: " . json_encode($res, JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE) . "\n";
} catch (SMSException $e) {
    fprintf(STDERR, "FAILED [%s]: %s\n", $e->code, $e->getMessage());
    exit(1);
}
