from datetime import date


def today() -> str:
    return date.today().isoformat()
