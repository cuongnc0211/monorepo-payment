require "net/http"
require "json"

module NemPay
  # Thin HTTP client for the NemPay /v1 API. Talks HTTP only (no shared DB); every money-mutating
  # POST carries a caller-supplied Idempotency-Key. Business declines come back as HTTP 200 with an
  # intent status of "failed" — they are NOT transport errors, so the caller inspects `status`.
  class Client
    OPEN_TIMEOUT = 5   # seconds
    READ_TIMEOUT = 15  # generous: the gateway may block briefly on the bank

    def initialize(base_url: NemPay.api_url, secret_key: NemPay.secret_key)
      @base_url = base_url
      @secret_key = secret_key
    end

    def create_intent(amount_cents:, currency:, metadata:, idempotency_key:)
      # Direct mode only: NO escrow flag, NO payee — just amount, currency, opaque metadata.
      post("/v1/payment_intents",
           { amount: amount_cents, currency: currency, metadata: metadata },
           idempotency_key: idempotency_key)
    end

    def confirm(intent_id:, token:, idempotency_key:)
      post("/v1/payment_intents/#{intent_id}/confirm", { token: token }, idempotency_key: idempotency_key)
    end

    def capture(intent_id:, idempotency_key:)
      post("/v1/payment_intents/#{intent_id}/capture", {}, idempotency_key: idempotency_key)
    end

    def get_intent(intent_id)
      request(Net::HTTP::Get.new(uri_for("/v1/payment_intents/#{intent_id}")))
    end

    private

    def post(path, payload, idempotency_key:)
      req = Net::HTTP::Post.new(uri_for(path))
      req["Content-Type"] = "application/json"
      req["Idempotency-Key"] = idempotency_key
      req.body = JSON.generate(payload)
      request(req)
    end

    def uri_for(path)
      URI.join(@base_url, path)
    end

    def request(req)
      req["Authorization"] = "Bearer #{@secret_key}"
      uri = req.uri
      res = Net::HTTP.start(uri.hostname, uri.port,
                            use_ssl: uri.scheme == "https",
                            open_timeout: OPEN_TIMEOUT, read_timeout: READ_TIMEOUT) do |http|
        http.request(req)
      end
      parse(res)
    rescue StandardError => e
      # Timeouts and connection failures are "unknown outcome" transport errors.
      Result.error(error: e.message)
    end

    def parse(res)
      code = res.code.to_i
      body = res.body.to_s.empty? ? {} : JSON.parse(res.body)
      if code.between?(200, 299)
        Result.ok(intent_id: body["id"], status: body["status"], body: body, status_code: code)
      else
        Result.error(status_code: code, body: body, error: body.dig("error", "message") || "gateway error #{code}")
      end
    rescue JSON::ParserError => e
      Result.error(error: "invalid gateway response: #{e.message}")
    end
  end
end
