FROM python:3.13-slim
WORKDIR /app
COPY server/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt
COPY server /app/server
COPY analysis/tables /app/analysis/tables
EXPOSE 18080 19090
CMD ["python", "-m", "server.app", "--bind", "0.0.0.0", "--db", "/data/local.sqlite3"]
