import { OrdersWidget } from "../widgets/orders"
import { MissingWidget } from "../missing/widget"

export const OrdersPage = () => `${OrdersWidget()}${MissingWidget}`
