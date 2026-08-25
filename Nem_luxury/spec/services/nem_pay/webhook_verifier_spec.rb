require 'rails_helper'

RSpec.describe NemPay::WebhookVerifier do
  let(:secret) { "whsec_test" }
  let(:body) { '{"id":"pi_1","status":"captured"}' }
  let(:good) { "sha256=" + OpenSSL::HMAC.hexdigest("SHA256", secret, body) }

  it "accepts a signature computed over the exact body with the shared secret" do
    expect(described_class.verify(body, good, secret)).to be(true)
  end

  it "rejects a tampered body" do
    expect(described_class.verify(body + " ", good, secret)).to be(false)
  end

  it "rejects the wrong secret" do
    expect(described_class.verify(body, good, "other")).to be(false)
  end

  it "rejects an empty signature or secret" do
    expect(described_class.verify(body, "", secret)).to be(false)
    expect(described_class.verify(body, good, "")).to be(false)
  end
end
