import { Button } from "../components/button";
import { listOrders } from "../lib/services/orders";

export function Layout() {
  return Button(listOrders);
}
