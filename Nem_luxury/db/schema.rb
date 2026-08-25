# This file is auto-generated from the current state of the database. Instead
# of editing this file, please use the migrations feature of Active Record to
# incrementally modify your database, and then regenerate this schema definition.
#
# This file is the source Rails uses to define your schema when running `bin/rails
# db:schema:load`. When creating a new database, `bin/rails db:schema:load` tends to
# be faster and is potentially less error prone than running all of your
# migrations from scratch. Old migrations may fail to apply correctly if those
# migrations use external dependencies or application code.
#
# It's strongly recommended that you check this file into your version control system.

ActiveRecord::Schema[8.1].define(version: 2026_08_25_042012) do
  create_table "orders", force: :cascade do |t|
    t.integer "amount_cents", null: false
    t.string "checkout_token", null: false
    t.datetime "created_at", null: false
    t.string "currency", limit: 3, null: false
    t.string "nem_pay_intent_id"
    t.integer "product_id", null: false
    t.integer "status", default: 0, null: false
    t.datetime "updated_at", null: false
    t.index ["checkout_token"], name: "index_orders_on_checkout_token", unique: true
    t.index ["product_id"], name: "index_orders_on_product_id"
  end

  create_table "processed_webhook_events", force: :cascade do |t|
    t.datetime "created_at", null: false
    t.string "event_id", null: false
    t.string "event_type", null: false
    t.integer "order_id"
    t.index ["event_id"], name: "index_processed_webhook_events_on_event_id", unique: true
    t.index ["order_id"], name: "index_processed_webhook_events_on_order_id"
  end

  create_table "products", force: :cascade do |t|
    t.integer "amount_cents", null: false
    t.datetime "created_at", null: false
    t.string "currency", limit: 3, null: false
    t.text "description"
    t.string "name", null: false
    t.datetime "updated_at", null: false
  end

  add_foreign_key "orders", "products"
  add_foreign_key "processed_webhook_events", "orders"
end
