from typing import Protocol

class OrderRepository(Protocol):
    def find(self, identifier: str) -> "Order":
        ...
