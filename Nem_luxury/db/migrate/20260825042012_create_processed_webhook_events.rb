class CreateProcessedWebhookEvents < ActiveRecord::Migration[8.1]
  def change
    create_table :processed_webhook_events do |t|
      t.string     :event_id, null: false                       # NemPay's stable event id (dedup key)
      t.string     :event_type, null: false
      t.references :order, null: true, foreign_key: true        # nil when the intent is unknown to us

      t.datetime :created_at, null: false
    end
    # The dedup guarantee: at-least-once delivery can replay an event; the UNIQUE index makes the
    # second insert fail, so the handler processes each event exactly once.
    add_index :processed_webhook_events, :event_id, unique: true
  end
end
