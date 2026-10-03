# Shell Chat Analytics & Agentic Service

This is a standalone Python microservice designed to handle background data processing, real-time analytics, and agentic AI integrations for the Shell Chat platform.

## Architecture

This service demonstrates a modern, event-driven microservices architecture:
1. **Event Streaming (Kafka)**: It consumes real-time chat messages directly from the Apache Kafka event stream, operating completely asynchronously from the main Go backend.
2. **Real-time Analytics**: Messages are aggregated (e.g., message velocity, sentiment) and stored in a PostgreSQL database using complex SQL upserts.
3. **REST API**: It exposes RESTful endpoints (via FastAPI) that other services or dashboards can query to retrieve analytics data.

## Tech Stack
- **Python 3.10+**
- **FastAPI** (REST Microservices)
- **Confluent Kafka** (Distributed Systems & Data Processing)
- **Psycopg2 / PostgreSQL** (Strong SQL Skills)

## Getting Started

```bash
# 1. Install dependencies
pip install -r requirements.txt

# 2. Run the microservice
python main.py
```
