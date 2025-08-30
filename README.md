# PDF Document Processing & AI Search API

A Go/Fiber web application that extracts text from PDF documents, stores them in a Qdrant vector database, and provides AI-powered search and question answering capabilities using OpenRouter API.

## Features

- **Document Processing**: Extract text from PDF, DOCX, ODT, and DOC files
- **Vector Database**: Store document content in Qdrant with OpenAI embeddings
- **Hybrid Search**: Combine semantic and keyword-based search
- **AI-Powered Q&A**: Answer questions using OpenRouter's Gemini Flash 1.5 8B model
- **Context Preservation**: 20% paragraph overlap for better context
- **User Isolation**: Per-user document storage and search
- **Keyword Extraction**: AI-powered keyword extraction for better search

## Environment Variables

```bash
# Required
OPENAI_API_KEY=your_openai_api_key_here
QDRANT_URL=your_qdrant_url_here
QDRANT_API_KEY=your_qdrant_api_key_here
OPENROUTER_API_KEY=your_openrouter_api_key_here

# Optional
PORT=3000  # Default port, Railway sets this automatically
```

## API Endpoints

### Health Check
```
GET /health
```
Returns server status.

### Document Processing

#### Extract Text from Document
```
POST /extract
Content-Type: multipart/form-data

Body:
- file: PDF/DOCX/ODT/DOC file
```

**Response:**
```json
{
  "pages": ["page 1 content", "page 2 content", ...],
  "filename": "document.pdf"
}
```

#### Extract and Store in Vector Database
```
POST /extract/store
Content-Type: multipart/form-data

Body:
- file: PDF/DOCX/ODT/DOC file
- username: string (required)
- doc_name: string (optional, defaults to filename)
```

**Response:**
```json
{
  "success": true,
  "pages_stored": 15,
  "doc_name": "my_document.pdf",
  "username": "john_doe"
}
```

### Search & Retrieval

#### Search Documents
```
POST /search
Content-Type: application/json

Body:
{
  "username": "john_doe",
  "query": "search terms",
  "doc_name": "specific_document.pdf", // optional
  "limit": 10 // optional, default 10
}
```

**Response:**
```json
{
  "success": true,
  "results": [
    {
      "id": "unique_id",
      "score": 0.95,
      "payload": {
        "username": "john_doe",
        "text": "document content...",
        "page_num": 1,
        "doc_name": "document.pdf"
      }
    }
  ],
  "total_found": 5
}
```

### AI-Powered Features

#### Answer Questions with AI
```
POST /answer
Content-Type: application/json

Body:
{
  "username": "john_doe",
  "question": "What is the main topic discussed?",
  "doc_name": "specific_document.pdf", // optional
  "limit": 5 // optional, default 5
}
```

**Response:**
```json
{
  "success": true,
  "answer": "Based on the documents, the main topic discussed is...",
  "sources_found": 3,
  "search_results": [
    {
      "id": "unique_id",
      "score": 0.95,
      "payload": {
        "username": "john_doe",
        "text": "relevant content...",
        "page_num": 1,
        "doc_name": "document.pdf"
      }
    }
  ]
}
```

#### Extract Keywords
```
POST /extract-keywords
Content-Type: application/json

Body:
{
  "query": "I want to find information about machine learning algorithms"
}
```

**Response:**
```json
{
  "success": true,
  "keywords": "machine learning algorithms artificial intelligence neural networks"
}
```

#### Smart Search (All-in-One)
```
POST /smart-search
Content-Type: application/json

Body:
{
  "username": "john_doe",
  "query": "I want to understand machine learning concepts",
  "doc_name": "specific_document.pdf", // optional
  "limit": 5 // optional, default 5
}
```

**Response:**
```json
{
  "success": true,
  "answer": "Machine learning is a subset of artificial intelligence that...",
  "keywords_extracted": "machine learning concepts artificial intelligence algorithms",
  "enhanced_query": "I want to understand machine learning concepts machine learning concepts artificial intelligence algorithms",
  "sources_found": 4,
  "search_results": [
    {
      "id": "unique_id",
      "score": 0.98,
      "payload": {
        "username": "john_doe",
        "text": "relevant content about machine learning...",
        "page_num": 3,
        "doc_name": "document.pdf"
      }
    }
  ]
}
```

### User Management

#### Delete User Data
```
DELETE /leave/:username
```

**Response:**
```json
{
  "success": true,
  "username": "john_doe",
  "deleted_count": 25,
  "message": "All user data deleted successfully"
}
```

## Technical Details

### Vector Database
- **Engine**: Qdrant
- **Embeddings**: OpenAI text-embedding-3-small (1536 dimensions)
- **Search Type**: Hybrid (semantic + keyword)
- **Context Preservation**: 20% overlap between consecutive paragraphs

### AI Integration
- **Provider**: OpenRouter
- **Model**: Gemini Flash 1.5 8B
- **Languages**: Romanian and English support
- **Features**: Question answering and keyword extraction

### Document Processing
- **Supported Formats**: PDF, DOCX, ODT, DOC
- **Text Extraction**: Intelligent page splitting with context preservation
- **Metadata**: Page numbers, document names, user isolation

## Deployment

### Railway
The application is configured for Railway deployment with:
- Automatic PORT detection
- Docker containerization
- Environment variable management

### Local Development
```bash
# Install dependencies
go mod tidy

# Set environment variables in .env file

# Run the application
go run .
```

## Error Handling

All endpoints return consistent error responses:
```json
{
  "success": false,
  "error": "Detailed error message"
}
```

## Security Considerations

- User data is isolated by username
- API keys are required for external services
- File upload validation and size limits
- Input sanitization and validation

## Performance Features

- Batch processing for large documents
- Optimized vector storage with overlap
- Efficient hybrid search algorithms
- Background processing for long operations
