from dataclasses import dataclass

from app.adapters.memory import MemoryRepository


@dataclass
class Order:
    identifier: str


def load_order(identifier: str) -> Order:
    return MemoryRepository().find(identifier)
