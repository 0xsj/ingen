from dataclasses import dataclass

from ..adapters.clock import today


@dataclass
class Order:
    identifier: str


def new_order() -> Order:
    return Order(identifier=today())
