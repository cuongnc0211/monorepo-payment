class CreateOrders < ActiveRecord::Migration[8.1]
  def change
    create_table :orders do |t|
      t.references :product, null: false, foreign_key: true
      t.integer :amount_cents, null: false            # snapshot of the price at checkout
      t.string  :currency, null: false, limit: 3
      t.integer :status, null: false, default: 0      # 0 = pending_payment (see Order enum)
      t.string  :checkout_token, null: false          # per-checkout natural key (double-submit guard)
      t.string  :nem_pay_intent_id                    # set once the NemPay intent is created

      t.timestamps
    end
    # One order per checkout attempt: a double-submit maps to the same row, not a second charge.
    add_index :orders, :checkout_token, unique: true
  end
end
