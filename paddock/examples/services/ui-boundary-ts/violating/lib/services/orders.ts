import { Button } from "../../components/button";
import type { HttpClient } from "../http/client";
import type { Order } from "../kernel/types";

export async function listOrders(client: HttpClient): Promise<Order[]> {
  Button(client);
  return (await client.get("/orders")) as Order[];
}
