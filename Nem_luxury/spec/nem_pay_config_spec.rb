require 'rails_helper'

# Smoke coverage for task-01: the app boots and NemPay configuration resolves from the
# environment (secrets from ENV, api_url with a local default).
RSpec.describe 'NemPay configuration' do
  it 'resolves the api url, secret key and webhook secret from the environment' do
    expect(NemPay.api_url).to be_present
    expect(NemPay.secret_key).to eq(ENV.fetch('NEMPAY_SECRET_KEY'))
    expect(NemPay.webhook_secret).to eq(ENV.fetch('NEMPAY_WEBHOOK_SECRET'))
  end

  it 'raises rather than inventing a secret when NEMPAY_SECRET_KEY is unset' do
    original = ENV.delete('NEMPAY_SECRET_KEY')
    expect { NemPay.secret_key }.to raise_error(KeyError)
  ensure
    ENV['NEMPAY_SECRET_KEY'] = original
  end
end
