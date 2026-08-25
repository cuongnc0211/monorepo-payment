# One row per NemPay webhook event we've handled. The UNIQUE event_id is the dedup point for
# at-least-once delivery: a replayed event fails to insert, so its side effect runs only once.
class ProcessedWebhookEvent < ApplicationRecord
  belongs_to :order, optional: true

  # Deliberately NO uniqueness validation on event_id — dedup is insert-first against the DB UNIQUE
  # index (the handler rescues RecordNotUnique). A model-level uniqueness check would SELECT-then-
  # insert (a TOCTOU race) and would surface a duplicate as RecordInvalid, defeating the pattern.
  validates :event_id, presence: true
  validates :event_type, presence: true
end
