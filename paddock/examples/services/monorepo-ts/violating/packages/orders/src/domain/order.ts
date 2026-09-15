import { makeID, type ID } from "@paddock/shared/id";

export interface Order {
  id: ID;
}

export function newOrder(): Order {
  return { id: makeID("order-1") };
}
