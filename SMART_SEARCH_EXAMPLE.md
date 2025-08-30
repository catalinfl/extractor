# Smart Search API Example

This example demonstrates how to use the new `/smart-search` endpoint that combines AI keyword extraction, vector database search, and AI-powered answers in a single request.

## What does `/smart-search` do?

1. **Keyword Extraction**: Uses OpenRouter AI (Gemini Flash 1.5 8B) to extract relevant keywords from your natural language query
2. **Enhanced Search**: Combines your original query with extracted keywords for better search results in Qdrant vector database
3. **AI Answer**: Generates a comprehensive answer based on the search results using OpenRouter AI

## Example Usage

### Request
```bash
curl -X POST http://localhost:3000/smart-search \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john_doe",
    "query": "I want to understand how neural networks learn from data",
    "limit": 3
  }'
```

### Response
```json
{
  "success": true,
  "answer": "Neural networks learn from data through a process called training, where they adjust their internal parameters (weights and biases) based on the patterns they discover in the training data. This process involves feeding the network examples, calculating the error between predicted and actual outputs, and using backpropagation to update the weights to minimize this error.",
  "keywords_extracted": "neural networks learning data training algorithms backpropagation",
  "enhanced_query": "I want to understand how neural networks learn from data neural networks learning data training algorithms backpropagation",
  "sources_found": 3,
  "search_results": [
    {
      "id": "doc_123_page_5",
      "score": 0.94,
      "payload": {
        "username": "john_doe",
        "text": "[Page 5, Paragraph 2/3]\nNeural networks are computational models inspired by biological neural networks. They learn patterns from data through a process called training, where the network adjusts its weights based on the error between predicted and actual outputs...",
        "page_num": 5,
        "doc_name": "ml_basics.pdf"
      }
    },
    {
      "id": "doc_123_page_12",
      "score": 0.89,
      "payload": {
        "username": "john_doe",
        "text": "[Page 12, Paragraph 1/4]\nBackpropagation is the key algorithm used to train neural networks. It calculates gradients of the loss function with respect to the network's weights and uses these gradients to update the parameters...",
        "page_num": 12,
        "doc_name": "ml_basics.pdf"
      }
    },
    {
      "id": "doc_456_page_3",
      "score": 0.85,
      "payload": {
        "username": "john_doe",
        "text": "[Page 3, Paragraph 3/5]\nData is the fuel that powers machine learning algorithms. The quality and quantity of training data directly impacts how well a neural network can learn and generalize to new, unseen examples...",
        "page_num": 3,
        "doc_name": "data_science.pdf"
      }
    }
  ]
}
```

## Benefits of Smart Search

### 🎯 **Better Accuracy**
- AI extracts the most relevant keywords from natural language queries
- Enhanced search combines original intent with technical terms
- More precise results from vector database

### 🚀 **Faster Workflow**
- Single API call instead of multiple requests
- No need to manually extract keywords
- Immediate AI-powered answer

### 🧠 **Intelligent Processing**
- Understands context and intent from natural language
- Finds documents even when using different terminology
- Provides comprehensive answers with source citations

## Use Cases

### 1. **Research Assistant**
```json
{
  "username": "researcher123",
  "query": "What are the latest developments in quantum computing?",
  "doc_name": "research_papers_2024.pdf"
}
```

### 2. **Study Helper**
```json
{
  "username": "student456",
  "query": "I need to understand the basics of photosynthesis for my biology exam",
  "limit": 5
}
```

### 3. **Technical Documentation**
```json
{
  "username": "developer789",
  "query": "How do I implement authentication in my web application?",
  "doc_name": "web_dev_guide.pdf"
}
```

## Comparison with Other Endpoints

| Endpoint | Steps Required | Use Case |
|----------|---------------|----------|
| `/search` | 1 step | Basic vector search |
| `/extract-keywords` + `/search` + `/answer` | 3 separate calls | Manual workflow |
| `/smart-search` | 1 step | Complete AI-powered search & answer |

## Error Handling

```json
{
  "success": false,
  "error": "Failed to extract keywords: OpenRouter API error"
}
```

Common errors:
- Missing username or query
- OpenRouter API key not configured
- Qdrant database connection issues
- No documents found for user

## Tips for Best Results

1. **Be specific**: "explain machine learning algorithms" vs "what is ML"
2. **Use natural language**: The AI will extract technical terms automatically
3. **Set appropriate limits**: More results = more context but longer processing time
4. **Filter by document**: Use `doc_name` for focused searches

## Environment Setup

Make sure you have these environment variables configured:
```bash
OPENROUTER_API_KEY=your_openrouter_key_here
QDRANT_URL=your_qdrant_url_here
QDRANT_API_KEY=your_qdrant_key_here
OPENAI_API_KEY=your_openai_key_here
```
