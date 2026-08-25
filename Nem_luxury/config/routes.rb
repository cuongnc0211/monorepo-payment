Rails.application.routes.draw do
  root "products#index"

  resources :products, only: %i[index show]
  resources :orders, only: %i[show]

  # Checkout + inbound webhook are wired in task-03 / task-04.
  post "/checkout", to: "checkouts#create", as: :checkout
  post "/webhooks/nem_pay", to: "webhooks/nem_pay#create"

  # Rails health check.
  get "up" => "rails/health#show", as: :rails_health_check
end
