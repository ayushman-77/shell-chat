FROM python:3.10-slim

WORKDIR /app

# Install system dependencies required for building psycopg2 and confluent-kafka
RUN apt-get update && apt-get install -y \
    gcc \
    libpq-dev \
    librdkafka-dev \
    && rm -rf /var/lib/apt/lists/*

# Install python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY main.py .

# Expose FastAPI port
EXPOSE 8000

# Run the server
CMD ["python", "main.py"]
