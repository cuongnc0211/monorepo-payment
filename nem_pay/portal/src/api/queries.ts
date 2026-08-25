import { useQuery } from "@tanstack/react-query";
import { api } from "./client";

const PAGE = 20;

export function usePayments(status: string, page: number) {
  return useQuery({
    queryKey: ["payments", status, page],
    queryFn: async () => {
      const query: Record<string, string | number> = { limit: PAGE, offset: page * PAGE };
      if (status) query.status = status;
      const { data, error } = await api.GET("/v1/payment_intents", { params: { query } });
      if (error) throw new Error("failed to load payments");
      return data;
    },
  });
}

export function usePayment(id: string) {
  return useQuery({
    queryKey: ["payment", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/v1/payment_intents/{id}", { params: { path: { id } } });
      if (error) throw new Error("failed to load payment");
      return data;
    },
  });
}

export function usePaymentLedger(id: string) {
  return useQuery({
    queryKey: ["ledger", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/v1/payment_intents/{id}/ledger", { params: { path: { id } } });
      if (error) throw new Error("failed to load ledger");
      return data;
    },
  });
}

export function useBalances() {
  return useQuery({
    queryKey: ["balances"],
    queryFn: async () => {
      const { data, error } = await api.GET("/v1/balances", {});
      if (error) throw new Error("failed to load balances");
      return data;
    },
  });
}

export function useWebhookEvents(page: number) {
  return useQuery({
    queryKey: ["webhooks", page],
    queryFn: async () => {
      const { data, error } = await api.GET("/v1/webhook_events", { params: { query: { limit: PAGE, offset: page * PAGE } } });
      if (error) throw new Error("failed to load webhook events");
      return data;
    },
  });
}

export function useApiKeys() {
  return useQuery({
    queryKey: ["api_keys"],
    queryFn: async () => {
      const { data, error } = await api.GET("/v1/api_keys", {});
      if (error) throw new Error("failed to load API keys");
      return data;
    },
  });
}

export const PAGE_SIZE = PAGE;
