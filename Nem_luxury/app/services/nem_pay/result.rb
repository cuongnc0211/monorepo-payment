module NemPay
  # The outcome of a gateway call or a checkout attempt. Three outcomes:
  #   :ok       — succeeded (HTTP 2xx; for a checkout, driven through to capture)
  #   :declined — the bank declined (a business outcome, not an error)
  #   :error    — a transport/gateway failure (timeout, 5xx, unexpected 4xx)
  class Result
    attr_reader :outcome, :intent_id, :status, :body, :error, :status_code

    def initialize(outcome:, intent_id: nil, status: nil, body: nil, error: nil, status_code: nil)
      @outcome = outcome
      @intent_id = intent_id
      @status = status        # the payment-intent status from the gateway (e.g. "captured")
      @body = body
      @error = error
      @status_code = status_code
    end

    def self.ok(**kwargs)       = new(outcome: :ok, **kwargs)
    def self.declined(**kwargs) = new(outcome: :declined, **kwargs)
    def self.error(**kwargs)    = new(outcome: :error, **kwargs)

    def ok?       = outcome == :ok
    def declined? = outcome == :declined
    def error?    = outcome == :error
  end
end
