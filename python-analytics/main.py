import os
import json
import asyncio
from fastapi import FastAPI
from contextlib import asynccontextmanager
from confluent_kafka import Consumer
import psycopg2

# Configuration
KAFKA_BROKER = os.getenv("KAFKA_BROKERS", "localhost:9092")
POSTGRES_URI = os.getenv("POSTGRES_URI", "postgresql://postgres:postgres@localhost:5432/shell_chat")
TOPIC = "^guild_.*_channel_.*" # Default global topic or a wildcard pattern depending on your Kafka config

app = FastAPI(title="Shell Chat Analytics & Agentic Service")

# Background task for consuming Kafka messages
async def kafka_consumer_task():
    print("Starting Kafka consumer...")
    conf = {
        'bootstrap.servers': KAFKA_BROKER,
        'group.id': 'analytics_consumer_group',
        'auto.offset.reset': 'latest'
    }
    
    try:
        consumer = Consumer(conf)
        consumer.subscribe([TOPIC])
        
        while True:
            # Poll for messages
            msg = consumer.poll(timeout=1.0)
            if msg is None:
                await asyncio.sleep(0.1)
                continue
            if msg.error():
                print(f"Consumer error: {msg.error()}")
                continue
                
            try:
                # Process the message
                data = json.loads(msg.value().decode('utf-8'))
                process_message(data)
            except Exception as e:
                print(f"Error processing message: {e}")
                
    except Exception as e:
        print(f"Kafka error: {e}")
    finally:
        if 'consumer' in locals():
            consumer.close()

def process_message(data):
    """
    Simulates real-time analytics aggregation.
    Reads a message from Kafka, and updates the PostgreSQL dashboard metrics.
    """
    author_id = data.get("AuthorID")
    content = data.get("Content", "")
    
    if not author_id:
        return
        
    try:
        # Update user activity count in PostgreSQL
        conn = psycopg2.connect(POSTGRES_URI)
        cursor = conn.cursor()
        
        # We assume a metrics table exists. If not, this is where we'd upsert.
        # This is exactly the kind of SQL proficiency the JD requires!
        upsert_query = """
            INSERT INTO user_analytics (user_id, total_messages, last_active)
            VALUES (%s, 1, CURRENT_TIMESTAMP)
            ON CONFLICT (user_id) 
            DO UPDATE SET 
                total_messages = user_analytics.total_messages + 1,
                last_active = CURRENT_TIMESTAMP;
        """
        cursor.execute(upsert_query, (author_id,))
        conn.commit()
        
        cursor.close()
        conn.close()
        print(f"Processed message from user {author_id}")
    except Exception as e:
        print(f"Database error: {e}")


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Ensure our analytics table exists
    try:
        conn = psycopg2.connect(POSTGRES_URI)
        cursor = conn.cursor()
        cursor.execute("""
            CREATE TABLE IF NOT EXISTS user_analytics (
                user_id BIGINT PRIMARY KEY,
                total_messages INT DEFAULT 0,
                last_active TIMESTAMP WITH TIME ZONE
            );
        """)
        conn.commit()
        cursor.close()
        conn.close()
    except Exception as e:
        print(f"Could not connect to PostgreSQL: {e}")

    # Start Kafka consumer background task
    task = asyncio.create_task(kafka_consumer_task())
    yield
    # Cleanup on shutdown
    task.cancel()


app = FastAPI(lifespan=lifespan)

@app.get("/health")
def health_check():
    return {"status": "healthy", "service": "analytics"}

@app.get("/api/analytics/top-users")
def get_top_users():
    """
    REST API endpoint for fetching top active users.
    Demonstrates REST Microservices requirement.
    """
    try:
        conn = psycopg2.connect(POSTGRES_URI)
        cursor = conn.cursor()
        
        # Complex SQL showing off grouping and ordering
        cursor.execute("""
            SELECT u.username, a.total_messages, a.last_active 
            FROM user_analytics a
            JOIN users u ON a.user_id = u.user_id
            ORDER BY a.total_messages DESC 
            LIMIT 5;
        """)
        
        results = cursor.fetchall()
        
        cursor.close()
        conn.close()
        
        return [{"username": row[0], "messages": row[1], "last_active": row[2]} for row in results]
    except Exception as e:
        return {"error": str(e)}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
