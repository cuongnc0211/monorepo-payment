require 'rails_helper'

RSpec.describe ProcessedWebhookEvent do
  it "enforces event_id uniqueness at the DB level (insert-first dedup)" do
    ProcessedWebhookEvent.create!(event_id: "evt_1", event_type: "payment_intent.captured")
    expect {
      ProcessedWebhookEvent.create!(event_id: "evt_1", event_type: "payment_intent.captured")
    }.to raise_error(ActiveRecord::RecordNotUnique)
  end

  it "allows a nil order (unknown intent)" do
    expect(ProcessedWebhookEvent.new(event_id: "evt_2", event_type: "payment_intent.captured")).to be_valid
  end
end
