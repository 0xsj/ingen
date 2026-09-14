from app.domain.order import Order
from app.ports.repository import OrderRepository


class MemoryRepository(OrderRepository):
    def find(self, identifier: str) -> Order:
        return Order(identifier=identifier)
