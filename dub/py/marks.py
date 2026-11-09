"""Stress marks: "+" before the stressed vowel. The dub command applies the same rules in marks.go."""
import re


def fix_marks(text):
    """Repair what Claude left: a "+" not before a vowel, in a one-vowel word, in a word with ё, or a second one.
    Also drops a word said twice with nothing between ("это это"): the narrator's stumble, not the text."""
    vowels = 'аеиоуыэюя'
    text = re.sub('[\u200b-\u200d\ufeff]', '', text)
    text = re.sub(r'(?i)(?<![\w+])([\w+]+) \1(?![\w+])', r'\1', text)

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
