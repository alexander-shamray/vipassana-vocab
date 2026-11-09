"""Second opinion on the stress marks: RuAccent (dictionary and homograph model) against Claude's marks."""
import collections, json, os, re, sys
sys.stdout.reconfigure(encoding='utf-8')
from ruaccent import RUAccent
from lessons import at
from marks import fix_marks

acc = RUAccent()
acc.load(omograph_model_size='turbo3.1', use_dictionary=True, tiny_mode=False)
# Claude's own marks, also when dub marks has already rewritten full-ru.json.
source = at('full-ru.claude.json') if os.path.exists(at('full-ru.claude.json')) else at('full-ru.json')
ru = json.load(open(source, encoding='utf-8'))
WORD = re.compile(r'[а-яёА-ЯЁ+]+')
out = []
diff = collections.Counter()
for p in ru:
    claude = fix_marks(p.get('ru_marked', ''))
    ruacc = acc.process_all(claude.replace('+', ''))
    p['ruaccent'] = ruacc
    a, b = WORD.findall(claude), WORD.findall(ruacc)
    if [w.replace('+', '').lower().replace('ё', 'е') for w in a] != [w.replace('+', '').lower().replace('ё', 'е') for w in b]:
        p['misaligned'] = True
        print('misaligned', p['id'], claude[:60], '|', ruacc[:60], flush=True)
        continue
    for x, y in zip(a, b):
        if x.lower() != y.lower():
            diff[(x.lower(), y.lower())] += 1
json.dump(ru, open(at('full-ru-accent.json'), 'w', encoding='utf-8'), ensure_ascii=False, indent=0)
print('phrases', len(ru), 'misaligned', sum(bool(p.get('misaligned')) for p in ru))
print('word differences', sum(diff.values()), 'distinct', len(diff))
json.dump([[a, b, n] for (a, b), n in diff.most_common()], open(at('accent-diff.json'), 'w', encoding='utf-8'), ensure_ascii=False, indent=0)
for (a, b), n in diff.most_common(70):
    print(n, a, '|', b)
