"""Word comparison shared by the phrase checks."""
import re


def norm(text):
    return re.findall(r'[а-я]+', text.replace('+', '').lower().replace('ё', 'е'))


def distance(a, b):
    prev = list(range(len(b) + 1))
    for i, x in enumerate(a, 1):
        cur = [i]
        for j, y in enumerate(b, 1):
            cur.append(min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (x != y)))
        prev = cur
    return prev[-1]
