require 'rails_helper'

# The app boots and serves the Rails health check.
RSpec.describe 'Health', type: :request do
  it 'responds 200 on /up' do
    get '/up'
    expect(response).to have_http_status(:ok)
  end
end
