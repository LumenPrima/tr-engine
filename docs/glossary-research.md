# Glossary Lookup System for ASR Post-Correction

> Research findings for building a phonetic/fuzzy/semantic lookup system to correct P25/DMR radio transcriptions against a curated glossary of known terms.
>
> Date: 2026-03-25

## Problem Statement

Raw ASR output from P25/DMR radio contains systematic phonetic errors:
- **Street names**: "barkley" → "Barclay", "new bergh" → "Newburgh"  
- **Unit callsigns**: "engine for" → "Engine 4", "medic to" → "Medic 2"
- **Landmarks/agencies**: "waynes burg" → "Waynesburg", "monticello" → "Monticello"
- **10-codes**: "ten four" → "10-4" (handled by pattern matching, but edge cases exist)

We need a glossary of ~10,000–50,000 known-correct terms per county/system, queryable at ~50–100 lookups per transcription, integrated into an LLM correction pipeline. The LLM receives raw ASR text + candidate corrections and produces the final output.

## Constraints

- **Already running**: PostgreSQL 17 with pg_trgm, pgvector
- **Preference**: Minimize new infrastructure (no Elasticsearch cluster if avoidable)
- **Latency target**: <100ms total for all glossary lookups per call (batch-friendly)
- **Scale**: Tens of thousands of glossary entries per system, hundreds of calls/day

---

## 1. PostgreSQL Extensions (Recommended Foundation)

### What's Available

| Extension | Function | Best For |
|-----------|----------|----------|
| **pg_trgm** | Trigram similarity (character n-grams) | Typos, spelling variations, partial matches |
| **fuzzystrmatch** | Soundex, Metaphone, Double Metaphone, Levenshtein, Daitch-Mokotoff | Phonetic similarity ("sounds like") |
| **pgvector** | Vector similarity search (HNSW/IVFFlat indexes) | Semantic or phonetic embeddings |

### Combined Strategy (The "PostgreSQL-Only" Approach)

The key insight: **no single algorithm handles all ASR error types**. Combining them with a scoring function is far more effective.

```sql
-- Example: multi-signal glossary lookup
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE glossary (
    id SERIAL PRIMARY KEY,
    term TEXT NOT NULL,                    -- canonical form: "Barclay Street"
    term_lower TEXT GENERATED ALWAYS AS (lower(term)) STORED,
    category TEXT,                         -- 'street', 'unit', 'landmark', 'agency', '10code'
    aliases TEXT[],                        -- known alternate forms
    soundex_code TEXT GENERATED ALWAYS AS (soundex(lower(term))) STORED,
    dmetaphone_pri TEXT GENERATED ALWAYS AS (dmetaphone(lower(term))) STORED,
    dmetaphone_alt TEXT GENERATED ALWAYS AS (dmetaphone_alt(lower(term))) STORED,
    embedding vector(384),                 -- phonetic or semantic embedding
    county_id INTEGER,                     -- scope to county/system
    source TEXT                            -- 'osm', 'radioreference', 'manual'
);

-- Indexes for each signal
CREATE INDEX idx_glossary_trgm ON glossary USING gin (term_lower gin_trgm_ops);
CREATE INDEX idx_glossary_soundex ON glossary (soundex_code);
CREATE INDEX idx_glossary_dmetaphone ON glossary (dmetaphone_pri);
CREATE INDEX idx_glossary_embedding ON glossary USING hnsw (embedding vector_cosine_ops);

-- Combined lookup function
CREATE OR REPLACE FUNCTION glossary_lookup(
    query_text TEXT,
    p_county_id INTEGER,
    p_limit INTEGER DEFAULT 5
)
RETURNS TABLE(term TEXT, category TEXT, combined_score FLOAT) AS $$
    SELECT
        g.term,
        g.category,
        (
            -- Trigram similarity (0-1, good for character-level errors)
            0.3 * similarity(g.term_lower, lower(query_text))
            -- Double Metaphone match (binary, good for phonetic errors)
            + 0.4 * CASE 
                WHEN g.dmetaphone_pri = dmetaphone(lower(query_text)) THEN 1.0
                WHEN g.dmetaphone_alt = dmetaphone(lower(query_text)) THEN 0.8
                WHEN g.dmetaphone_pri = dmetaphone_alt(lower(query_text)) THEN 0.8
                ELSE 0.0
              END
            -- Levenshtein distance (normalized, catches remaining errors)
            + 0.3 * (1.0 - least(levenshtein(g.term_lower, lower(query_text))::float 
                      / greatest(length(g.term_lower), length(lower(query_text)), 1), 1.0))
        ) AS combined_score
    FROM glossary g
    WHERE g.county_id = p_county_id
      AND (
          g.term_lower % lower(query_text)                              -- trigram filter
          OR g.soundex_code = soundex(lower(query_text))                -- soundex filter
          OR g.dmetaphone_pri = dmetaphone(lower(query_text))           -- dmetaphone filter
      )
    ORDER BY combined_score DESC
    LIMIT p_limit;
$$ LANGUAGE sql STABLE;
```

### Performance Characteristics

- **pg_trgm with GIN index**: ~1-5ms per query on 50K rows. Nearly as fast as exact match.
- **soundex/dmetaphone lookups on indexed columns**: <1ms (simple B-tree equality).
- **Levenshtein**: Expensive without pre-filtering (~50ms on 50K rows). Use ONLY on candidates pre-filtered by trigram or phonetic match.
- **pgvector HNSW**: ~1-5ms for ANN search on 50K vectors. Scales well.
- **Combined query with pre-filters**: Expect 5-15ms per lookup. For 50-100 lookups per call, that's 250ms-1.5s total **if sequential**. Batch with `unnest()` to bring total under 100ms.

### Strengths
- Zero new infrastructure
- All signals combined in one query
- ACID-compliant, transactional, easy to update glossary
- Pre-filtering (trigram OR soundex OR dmetaphone) keeps expensive calculations fast

### Weaknesses
- Double Metaphone is English-centric (fine for US P25/DMR)
- No purpose-built phonetic embeddings (see Section 4 for enhancement)
- Multi-word terms need tokenization strategy (search each word separately, then combine)

### Verdict: ⭐⭐⭐⭐⭐ — **Start here. This covers 80-90% of cases with zero new infrastructure.**

---

## 2. Dedicated Fuzzy/Phonetic Search Tools

### Elasticsearch with Phonetic Analysis Plugin

**How it works**: Token filters convert text to phonetic codes (Soundex, Metaphone, Double Metaphone, NYSIIS, Beider-Morse, Caverphone) at index time. Queries match against these codes.

**Advantages over PostgreSQL**:
- Beider-Morse Phonetic Matching (BMPM) — handles multilingual names better than Double Metaphone
- Built-in multi-field search with boosting (combine phonetic + trigram + exact in one query)
- Better relevance scoring out of the box (BM25 + custom scoring)
- Horizontal scalability (irrelevant at our scale)

**Disadvantages**:
- Requires running and maintaining Elasticsearch cluster
- Data synchronization from PostgreSQL (glossary changes need to propagate)
- Operational overhead significantly outweighs marginal accuracy improvement
- At 50K entries, PostgreSQL is not the bottleneck

### MeiliSearch / Typesense

Both are lightweight search engines with built-in typo tolerance. Neither has native phonetic analysis. Their fuzzy matching is character-distance-based (similar to pg_trgm), not phonetic. **Not significantly better for ASR errors specifically.**

### Apache Solr

Similar capabilities to Elasticsearch (same Lucene foundation). Even more operational overhead. Not recommended over ES if you were going that route.

### Verdict: ⭐⭐⭐ — **Not worth the infrastructure cost at our scale.** The marginal accuracy gain from Beider-Morse over Double Metaphone doesn't justify running another service. If you outgrow PostgreSQL or need multi-language support, Elasticsearch is the upgrade path.

---

## 3. Specialized ASR Correction Approaches

### 3a. LLM + RAG for Named Entity Correction (Most Relevant)

This is the state-of-the-art approach and **exactly matches our architecture**:

1. ASR produces raw text
2. **Retrieve** candidate corrections from glossary (phonetic + fuzzy matching)
3. **Augment** LLM prompt with candidates
4. LLM produces corrected text

Key research:
- **Apple's "Retrieval-Augmented ASR" (2024)**: Uses vector database to index entities, retrieves candidates based on ASR hypothesis, feeds to LLM for correction. Showed significant improvement on named entity WER.
- **Amazon's RECOVER framework (2024)**: Multiple ASR hypotheses + entity retrieval + constrained LLM correction. Demonstrated major reduction in entity-phrase word error rates.
- **"Generative Annotation for ASR Named Entity Correction" (EMNLP 2025)**: Uses speech sound features to retrieve candidate entities, generative model annotates and replaces incorrect entities. Open-sourcing training data and test sets.

**This is what we should build.** The glossary lookup function IS the retrieval step. The LLM correction pipeline IS the generation step. We're already doing RAG for ASR correction — we just need to formalize and optimize the retrieval.

### 3b. Hotword Boosting / Contextual Biasing (Already Partially Using)

This operates at the **ASR decode level**, before post-correction:
- CTC beam search with external language model + hotword list
- Whisper/Qwen3-ASR can be biased with prompts containing expected terms
- More effective at decode time but requires integration with the ASR model

**We already use this partially** with Qwen3-ASR. Worth expanding the hotword list from the glossary, but this is complementary to post-correction, not a replacement.

### 3c. Weighted Finite-State Transducers (WFSTs)

Classical approach from Kaldi-era ASR. WFSTs compose a phoneme-to-word transducer with a word-level language model. Can be constrained to only output glossary terms for entity slots.

**Verdict**: Powerful but requires significant expertise in WFST composition. More relevant for building a custom ASR decoder than for post-correction. **Skip for now.**

### 3d. Open-Source Tools

| Tool | Description | Relevance |
|------|-------------|-----------|
| **PostASR-Correction-SLT2024** | Genetic algorithm for optimizing LLM prompts for ASR error correction | Interesting for prompt optimization |
| **NeMo (NVIDIA)** | Full ASR toolkit with contextual biasing support | Overkill for post-correction |
| **Epitran** | Grapheme-to-phoneme for 100+ languages | Useful for generating phonetic representations |
| **Panphon** | Phonological feature vectors for phonemes | Could generate phonetic embeddings |
| **g2p-en** | English grapheme-to-phoneme | Lightweight G2P for generating phoneme sequences |

### Verdict: ⭐⭐⭐⭐⭐ — **The RAG approach (3a) is exactly what we need. Formalize the retrieval step with the PostgreSQL glossary lookup. Expand hotword boosting (3b) as complementary.**

---

## 4. Phonetic Embedding Models for Vector Similarity

### The Idea

Instead of (or in addition to) algorithmic phonetic matching (Metaphone etc.), encode words as vectors where phonetically similar words are close together. Store in pgvector, search with ANN.

### Available Approaches

#### A. Phoneme-based embeddings (Best fit)
1. Convert text to phonemes using G2P (e.g., `epitran`, `g2p-en`)
2. Generate embeddings from phoneme sequences using:
   - Character-level models trained on phoneme strings
   - Siamese networks trained on phoneme pairs
   - Simple approach: use a small sentence-transformer on the IPA/ARPAbet representation

**Example pipeline**:
```
"Barclay" → G2P → "B AA R K L EY" → embed → [0.23, -0.14, ...]
"barkley" → G2P → "B AA R K L IY" → embed → [0.22, -0.15, ...]
// cosine similarity ≈ 0.97
```

#### B. Acoustic word embeddings
Trained on actual audio to produce vectors where words that sound similar (in real speech, with noise, accents) are close. Research models exist (Wav2Vec2-based) but not readily available as off-the-shelf tools.

#### C. General text embeddings (e.g., sentence-transformers)
These capture **semantic** similarity, not phonetic. "Barclay" and "barkley" might not be close because they mean different things. **Not suitable for phonetic matching.**

#### D. Articulatory feature vectors (Panphon)
Panphon represents each phoneme as a vector of articulatory features (place of articulation, manner, voicing, etc.). Word-level vectors can be computed by averaging or sequencing phoneme vectors. Lightweight, interpretable, no model training needed.

### Recommended Approach: Panphon + pgvector

```python
import panphon
import panphon.distance
import numpy as np

ft = panphon.FeatureTable()

def phonetic_embedding(word: str, dim: int = 384) -> np.ndarray:
    """Generate a fixed-size phonetic embedding from articulatory features."""
    # Get IPA representation
    ipa = epitran.transliterate(word)  # or use g2p
    # Get feature vectors for each phoneme segment
    segments = ft.word_fts(ipa)
    if not segments:
        return np.zeros(dim)
    # Stack and pad/truncate to fixed size
    features = np.array([seg.numeric() for seg in segments])
    # Average pooling + positional info
    embedding = np.zeros(dim)
    # ... (implementation details: pad, position-encode, normalize)
    return embedding
```

This gives us:
- **No model training required** — deterministic, based on linguistic features
- **Phonetically meaningful** — similar sounds → similar vectors by construction
- **Fast** — no GPU needed, pure computation
- **Storable in pgvector** — indexed with HNSW for fast ANN search

### Performance for ASR Errors

| Error Type | Trigram | Metaphone | Phonetic Embedding | Combined |
|-----------|---------|-----------|-------------------|----------|
| "barkley" → "Barclay" | 0.6 | ✅ match | 0.95+ | ✅ |
| "engine for" → "Engine 4" | 0.3 | ❌ | 0.4 | Needs special handling* |
| "waynes burg" → "Waynesburg" | 0.7 | ❌ (tokenized differently) | 0.85 | ✅ |
| "new bergh" → "Newburgh" | 0.5 | ❌ | 0.80 | ✅ |
| "medic to" → "Medic 2" | 0.3 | ❌ | 0.3 | Needs special handling* |

*Number/word confusion ("for"→"4", "to"→"2") is a pattern-matching problem, not a phonetic one. Handle with regex rules or teach the LLM.

### Verdict: ⭐⭐⭐⭐ — **Worth adding as a signal in the combined PostgreSQL approach.** Panphon-based embeddings stored in pgvector add meaningful phonetic similarity that Double Metaphone misses (especially for multi-word terms and partial matches). But it's an enhancement, not a replacement for the algorithmic approach.

---

## 5. Hybrid / Production Approaches

### What Big Tech Does

| Company | Approach | Notes |
|---------|----------|-------|
| **Google (Voice Search)** | Contextual biasing at decode time + WFST composition with entity grammar + LLM reranking | Massive infrastructure, not directly applicable |
| **Apple (Siri)** | Retrieval-augmented correction: vector DB for entity retrieval + LLM post-correction | Closest to our architecture. Published 2024. |
| **Amazon (Alexa)** | RECOVER: multi-hypothesis + entity retrieval + constrained LLM correction | Also close to our approach. RAG-based. |
| **Samsung** | Dynamic biasing with locality-enhanced sampling | At decode time, not post-correction |

### The Emerging Pattern

The industry is converging on:
1. **Best-effort ASR** (with hotword boosting where possible)
2. **Entity retrieval** against a known vocabulary (the glossary lookup)
3. **LLM-based correction** with retrieved candidates as context

This is exactly what we're building. The question is just how to implement step 2 optimally.

### Production Architecture for tr-engine

```
┌──────────────────────────────────────────────────┐
│                  Call Audio                        │
│                     │                             │
│                     ▼                             │
│    ┌────────────────────────────────┐             │
│    │  Qwen3-ASR (with hotword list) │ ← Step 0   │
│    └────────────────────────────────┘             │
│                     │                             │
│              Raw transcript                       │
│                     │                             │
│                     ▼                             │
│    ┌────────────────────────────────┐             │
│    │  Extract candidate tokens      │ ← Step 1   │
│    │  (NER-like: names, numbers,    │             │
│    │   callsigns, locations)        │             │
│    └────────────────────────────────┘             │
│                     │                             │
│          Candidate tokens (50-100)                │
│                     │                             │
│                     ▼                             │
│    ┌────────────────────────────────┐             │
│    │  PostgreSQL Glossary Lookup    │ ← Step 2   │
│    │  • pg_trgm (trigram)           │             │
│    │  • Double Metaphone            │             │
│    │  • Phonetic embeddings (pgvec) │             │
│    │  • Combined scoring + ranking  │             │
│    └────────────────────────────────┘             │
│                     │                             │
│      Top-K candidates per token                   │
│                     │                             │
│                     ▼                             │
│    ┌────────────────────────────────┐             │
│    │  LLM Correction                │ ← Step 3   │
│    │  "Given raw transcript and     │             │
│    │   these glossary candidates,   │             │
│    │   produce corrected text"      │             │
│    └────────────────────────────────┘             │
│                     │                             │
│            Corrected transcript                   │
└──────────────────────────────────────────────────┘
```

---

## 6. Recommendations

### Phase 1: PostgreSQL Multi-Signal Lookup (Do Now)

**Effort**: 2-3 days | **Impact**: High | **Risk**: Low

1. **Enable fuzzystrmatch** extension (if not already)
2. **Add columns** to glossary table: `soundex_code`, `dmetaphone_pri`, `dmetaphone_alt` (generated/stored)
3. **Create combined lookup function** as shown in Section 1
4. **Add B-tree indexes** on phonetic code columns
5. **Batch lookups** using `unnest()` to reduce round trips
6. **Tune weights** (trigram vs phonetic vs Levenshtein) empirically against known ASR error pairs

This alone will catch the majority of phonetic errors with sub-100ms total latency per call.

### Phase 2: Phonetic Embeddings via pgvector (Next)

**Effort**: 3-5 days | **Impact**: Medium | **Risk**: Low

1. **Install epitran/panphon** or g2p-en for phoneme generation
2. **Generate phonetic embeddings** for all glossary entries
3. **Store in pgvector** column with HNSW index
4. **Add as additional signal** in combined scoring function
5. **Especially useful for**: multi-word terms, compound words, terms where Metaphone fails

### Phase 3: Expand Hotword Boosting (Complementary)

**Effort**: 1-2 days | **Impact**: Medium | **Risk**: Low

1. **Export glossary terms** as hotword list for Qwen3-ASR
2. **Weight by frequency** (common street names, frequent unit callsigns get higher boost)
3. **Per-system/county lists** to keep hotword set focused

### Phase 4: LLM Prompt Optimization (Ongoing)

**Effort**: Ongoing | **Impact**: High | **Risk**: Low

1. **Structure the correction prompt** to present glossary candidates clearly
2. **Include category hints** ("This appears to be a street name; candidates: Barclay St, Berkeley Ave, Barkley Ln")
3. **Add confidence scores** so the LLM can weigh uncertain vs certain corrections
4. **Consider few-shot examples** of common P25/DMR correction patterns

### Not Recommended (For Now)

| Approach | Why Not |
|----------|---------|
| Elasticsearch | Overkill for 50K entries. PostgreSQL handles this fine. |
| Custom WFST decoder | Requires deep ASR expertise, better to improve post-correction |
| Training custom phonetic embedding model | Panphon/G2P approach is good enough without training |
| MeiliSearch/Typesense | No phonetic analysis, only character-distance fuzzy matching |

---

## 7. Special Cases: Number/Word Confusion

ASR frequently confects numbers and homophones in P25 radio:
- "engine for" → "Engine 4"
- "medic to" → "Medic 2"  
- "ten seventy one" → "10-71"
- "channel won" → "Channel 1"

These are **not phonetic matching problems** — they're homophone disambiguation problems that require context. Handle with:

1. **Pattern rules**: `/{unit_type}\s+(one|two|three|four|five|...|won|to|too|for|ate)/` → convert to number
2. **LLM context**: The LLM already understands "engine for" likely means "Engine 4" in radio context
3. **Glossary entries with aliases**: Store "Engine 4" with aliases `["engine for", "engine four"]`

---

## 8. Multi-Word Term Matching Strategy

Many glossary entries are multi-word ("Barclay Street", "Engine Company 4", "Wayne County Sheriff"). ASR may split, merge, or mangle these differently.

**Recommended approach**:
1. **Index both full terms and individual significant words**
2. **For lookup**: search individual words, then check if adjacent words combine to match a multi-word term
3. **Sliding window**: for a 3-word window over the transcript, check if any 1/2/3-word combination matches a glossary entry
4. **The LLM handles the rest**: present candidates with their full canonical form; the LLM is good at reconstructing "waynes burg road" → "Waynesburg Road" given the candidate

---

## 9. Glossary Curation Pipeline

The glossary is only as good as its data. Sources:

| Source | Data Type | Update Frequency | Automation |
|--------|-----------|-------------------|------------|
| **OpenStreetMap** | Streets, landmarks, buildings, POIs | Weekly/monthly sync | Overpass API queries by county bbox |
| **RadioReference** | Talkgroups, agencies, unit designations | Monthly | Scrape/API (if available) |
| **FCC ULS** | Callsigns, license holders | Quarterly | FCC API |
| **Manual curation** | 10-codes, slang, local terms | As discovered | Admin UI or CSV import |
| **ASR error logs** | Discovered corrections | Continuous | Flag corrections in UI, batch import |

**Feedback loop**: When the LLM makes a correction that a human validates, add the raw→corrected pair to the glossary aliases. This makes the system learn from its corrections over time.

---

## Summary

| Approach | Accuracy | Latency | Complexity | LLM Integration | Recommendation |
|----------|----------|---------|------------|-----------------|----------------|
| PG multi-signal (trgm+metaphone+levenshtein) | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | **Do first** |
| + Phonetic embeddings (pgvector) | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | **Phase 2** |
| + Hotword boosting | ⭐⭐⭐⭐ | N/A (decode) | ⭐⭐⭐⭐⭐ | N/A | **Phase 3** |
| Elasticsearch | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ | Only if PG insufficient |
| Custom phonetic model | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ | Not worth it yet |
| WFST constrained decoding | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐ | ⭐⭐ | Skip |

**TL;DR**: PostgreSQL with pg_trgm + fuzzystrmatch (Double Metaphone) + combined scoring covers 80-90% of ASR phonetic errors. Add phonetic embeddings via pgvector/Panphon for the remaining edge cases. Feed candidates to the LLM with category hints and confidence scores. This is the RAG approach that Apple and Amazon are using in production — we just implement the retrieval step in PostgreSQL instead of a separate vector DB.
