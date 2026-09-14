from dataclasses import dataclass

from app.ports.repository import OrderRepository


@dataclass
class Order:
    identifier: str


def load_order(repository: OrderRepository, identifier: str) -> Order:
    return repository.find(identifier)
