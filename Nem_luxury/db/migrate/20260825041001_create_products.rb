class CreateProducts < ActiveRecord::Migration[8.1]
  def change
    create_table :products do |t|
      t.string  :name, null: false
      t.text    :description
      t.integer :amount_cents, null: false          # integer minor units, never floats
      t.string  :currency, null: false, limit: 3    # ISO-4217

      t.timestamps
    end
  end
end
