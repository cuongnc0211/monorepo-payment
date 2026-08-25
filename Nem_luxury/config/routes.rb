Rails.application.routes.draw do
  root "products#index"

  resources :products, only: %i[index show]
  resources :orders, only: %i[show]

  # Multi-step checkout: contact details → create order + intent → card page → pay.
  get  "/products/:product_id/checkout", to: "checkouts#new",    as: :new_checkout
  post "/checkout",                      to: "checkouts#create", as: :checkout
  get  "/orders/:id/payment",            to: "payments#new",     as: :order_payment
  post "/orders/:id/payment",            to: "payments#create",  as: :pay_order

  # Inbound webhook (task-04).
  post "/webhooks/nem_pay", to: "webhooks/nem_pay#create"

  # Rails health check.
  get "up" => "rails/health#show", as: :rails_health_check
end
