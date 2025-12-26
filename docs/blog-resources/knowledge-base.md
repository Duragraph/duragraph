# Blog Knowledge Base

> **Note**: This file is NOT published. It serves as a reference for blog content creation.
> Last updated: 2025-12-26

## AI Ecosystem Landscape

### Agent Orchestration Frameworks

| Framework | Maintainer | License | Key Feature | Best For |
|-----------|-----------|---------|-------------|----------|
| LangGraph | LangChain | MIT | Graph-based state machines | Complex workflows with explicit state |
| CrewAI | CrewAI | MIT | Role-based agents | Sequential/hierarchical task execution |
| AutoGen | Microsoft | MIT | Message-passing architecture | Auditable multi-agent systems |
| Haystack | deepset | Apache 2.0 | Pipeline-based NLP | Search, RAG, document QA |
| LlamaIndex | LlamaIndex | MIT | Data connectors | RAG and data-intensive apps |
| SuperAGI | SuperAGI | MIT | GUI management | Autonomous agent deployment |
| Semantic Kernel | Microsoft | MIT | Multi-language SDK | Enterprise Azure integration |

**Key Insight (2025)**: AutoGen is merging with Semantic Kernel into Microsoft Agent Framework (GA Q1 2026).

### Memory Agent Frameworks

| Framework | Approach | Key Feature | Accuracy |
|-----------|----------|-------------|----------|
| Mem0 | Graph-based | Relational memory structures | Research-backed |
| Letta (MemGPT) | Context management | Infinite conversation context | ADE visual workspace |
| Zep | Temporal KG | Graphiti engine, multi-layer memory | SOC 2 compliant |
| memU | Hybrid retrieval | Semantic + keyword + contextual | 92% Locomo |
| Memori | SQL/MongoDB | Database-native, no vendor lock-in | Production-ready |
| MemMachine | 4-mode memory | Episodic, semantic, procedural, profile | Cross-platform |

**Memory Types**:
- Episodic: What happened (events, conversations)
- Semantic: What it means (concepts, relationships)
- Procedural: How to do it (skills, workflows)
- Profile: Who am I working with (user context)

### LLM Observability Platforms

| Platform | Type | Key Feature | Scale |
|----------|------|-------------|-------|
| Langfuse | Open Source (MIT) | 78 features, full LLM engineering | Self-hostable |
| Helicone | Open Source | Cloudflare Workers architecture | 2B+ interactions |
| OpenLLMetry | OpenTelemetry | Standard OTel format, 10+ tool export | Framework-agnostic |
| Arize Phoenix | Open Source | Embeddings visualization | RAG debugging |
| TruLens | Open Source | Evaluation metrics | Quality scoring |

**June 2025 Update**: Langfuse open-sourced commercial modules (LLM-as-a-judge, annotation queues, prompt experiments, Playground) under MIT.

### LLM Gateways

| Gateway | Language | Latency | Models Supported |
|---------|----------|---------|------------------|
| LiteLLM | Python | 15-30min setup | 100+ |
| Portkey | TypeScript | ~50ms | 1,600+ |
| Bifrost (Maxim) | Go | <100µs @ 5k RPS | Multi-provider |
| Kong AI Gateway | - | Fastest (228% > Portkey) | Enterprise |
| Helicone Gateway | Rust | ~50ms | Multi-provider |

**Benchmark Note**: Kong AI Gateway is 859% faster than LiteLLM in benchmarks with 12 CPUs.

### LLM Inference Servers

| Server | Architecture | Best For | Key Tech |
|--------|--------------|----------|----------|
| vLLM | Python/CUDA | High-throughput serving | PagedAttention |
| TGI | Python | Production deployments | Hugging Face ecosystem |
| llama.cpp | C++ | Edge/CPU inference | GGUF, 2-8bit quantization |
| SGLang | Python | Advanced patterns | Tool use, multi-modal |
| TensorRT-LLM | C++/CUDA | Peak NVIDIA performance | Optimized kernels |
| Ollama | Go | Developer experience | Easy local setup |

**Performance Note**: vLLM delivers 24x higher throughput than HF Transformers via PagedAttention.

---

## Blog Post Ideas

### Comparison Posts
1. "CrewAI vs AutoGen vs LangGraph: Choosing Your Agent Framework in 2025"
2. "Memory Systems Showdown: Mem0 vs Letta vs Zep"
3. "LLM Gateway Benchmark: Speed, Cost, and Features Compared"
4. "vLLM vs llama.cpp: When to Use Each Inference Engine"

### Deep Dives
1. "Understanding PagedAttention: How vLLM Achieves 24x Throughput"
2. "Building Temporal Knowledge Graphs with Zep's Graphiti Engine"
3. "OpenTelemetry for LLMs: A Practical Guide to OpenLLMetry"
4. "The Microsoft Agent Framework: What AutoGen's Merger Means"

### Tutorials
1. "Migrating from LangGraph Cloud to DuraGraph"
2. "Adding Long-Term Memory to Your DuraGraph Agents with Mem0"
3. "Setting Up LLM Observability with Langfuse and DuraGraph"
4. "Cost-Effective LLM Routing with LiteLLM and DuraGraph"

### Ecosystem Updates
1. "Langfuse Goes Fully Open Source: What This Means for LLM Observability"
2. "The State of Agent Frameworks: 2025 Year in Review"
3. "Memory Agents in 2025: From Research to Production"

---

## Source Links

### Official Repositories
- CrewAI: https://github.com/joaomdmoura/crewAI
- AutoGen: https://github.com/microsoft/autogen
- Haystack: https://github.com/deepset-ai/haystack
- LlamaIndex: https://github.com/run-llama/llama_index
- Mem0: https://github.com/mem0ai/mem0
- Langfuse: https://github.com/langfuse/langfuse
- vLLM: https://github.com/vllm-project/vllm
- llama.cpp: https://github.com/ggerganov/llama.cpp
- LiteLLM: https://github.com/BerriAI/litellm
- Portkey: https://github.com/Portkey-AI/gateway

### Documentation
- CrewAI Docs: https://docs.crewai.com
- AutoGen Docs: https://microsoft.github.io/autogen/
- Haystack Docs: https://docs.haystack.deepset.ai
- Langfuse Docs: https://langfuse.com/docs
- vLLM Docs: https://docs.vllm.ai

### Research Papers
- Mem0 Paper: https://arxiv.org/abs/2504.19413
- Memory Survey: https://arxiv.org/abs/2512.13564
