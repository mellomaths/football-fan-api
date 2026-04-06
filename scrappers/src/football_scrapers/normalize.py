import re
import unicodedata


def normalize_team_name(name: str) -> str:
    """Lowercase ASCII fold for fuzzy matching against DB team names."""
    if not name:
        return ""
    n = unicodedata.normalize("NFKD", name)
    n = "".join(ch for ch in n if not unicodedata.combining(ch))
    n = n.lower().strip()
    n = re.sub(r"\s+", " ", n)
    return n
