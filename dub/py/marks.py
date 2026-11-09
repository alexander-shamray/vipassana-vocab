"""Stress marks: "+" before the stressed vowel. The dub command applies the same rules in marks.go."""
import json, re


def fix_marks(text):
    """Repair what Claude left: a "+" not before a vowel, in a one-vowel word, in a word with ё, or a second one.
    Also drops a word said twice with nothing between ("это это"): the narrator's stumble, not the text."""
    vowels = 'аеиоуыэюя'
    text = re.sub('[\u200b-\u200d\ufeff]', '', text)
    text = re.sub(r'(?i)(?<![\w+])([\w+]+) \1(?![\w+-])', r'\1', text)

    def word(m):
        w = m.group(0)
        low = w.lower()
        count = sum(c in vowels + 'ё' for c in low)
        if count < 2 or 'ё' in low:
            return w.replace('+', '')
        out, seen = [], False
        for i, c in enumerate(w):
            if c == '+':
                if not seen and i + 1 < len(w) and w[i + 1].lower() in vowels:
                    out.append(c)
                    seen = True
                continue
            out.append(c)
        return ''.join(out)

    return re.sub(r'[а-яёА-ЯЁ+]+', word, text)


def plain(s):
    return s.replace('+', '')


def xtts_text(marked):
    """The text XTTS reads: no marks and no quotation marks, which it voices as stray syllables ("Нет" becomes "Я нет").
    On lessons 1 and 4, phrases with quotation marks got extra words 60% of the time, the others about 15%."""
    return respell(re.sub('[«»„“”"]', '', plain(fix_marks(marked))))


_RESPELL = None


def respell(text):
    """XTTS has no stress mark: a word it misreads can only be spelled as it sounds. dub/xtts-respell.json, XTTS only."""
    global _RESPELL
    if _RESPELL is None:
        with open('../../dub/xtts-respell.json', encoding='utf-8') as f:
            _RESPELL = {k.lower(): v for k, v in json.load(f).items()}

    def one(m):
        new = _RESPELL.get(m.group(0).lower())
        if new is None:
            return m.group(0)
        return new[0].upper() + new[1:] if m.group(0)[0].isupper() else new

    return re.sub(r'[а-яёА-ЯЁ]+', one, text)
