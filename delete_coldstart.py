#!/usr/bin/env python3
"""Delete all records from the feed_coldstart table."""

import os

import psycopg2
from dotenv import load_dotenv

load_dotenv()

DB_CONFIG = {
    "host": os.getenv("DATABASE_HOST", "127.0.0.1"),
    "port": os.getenv("DATABASE_PORT", "5432"),
    "dbname": os.getenv("DATABASE_NAME", "apen"),
    "user": os.getenv("DATABASE_USERNAME", "postgres"),
    "password": os.getenv("DATABASE_PASSWORD", ""),
}


def delete_coldstart():
    conn = psycopg2.connect(**DB_CONFIG)
    cur = conn.cursor()

    try:
        cur.execute("DELETE FROM feed_coldstart")
        conn.commit()
        print(f"Deleted {cur.rowcount} records from feed_coldstart")
    except Exception as e:
        conn.rollback()
        raise e
    finally:
        cur.close()
        conn.close()


if __name__ == "__main__":
    delete_coldstart()
