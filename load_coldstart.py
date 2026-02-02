#!/usr/bin/env python3
"""Load coldstart.csv into the feed_coldstart table."""

import csv
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

CSV_FILE = "coldstart.csv"


def load_coldstart():
    conn = psycopg2.connect(**DB_CONFIG)
    cur = conn.cursor()

    try:
        with open(CSV_FILE, encoding="utf-8") as f:
            reader = csv.reader(f)
            next(reader)  # Skip header

            count = 0
            for position, row in enumerate(reader):
                if not row or not row[0].strip():
                    continue

                feed_id = row[0].strip()
                cur.execute(
                    """
                    INSERT INTO feed_coldstart (feed_id, feed_type, position)
                    VALUES (%s, %s, %s)
                    ON CONFLICT (feed_id) DO NOTHING
                    """,
                    (feed_id, "post", position),
                )
                count += 1

        conn.commit()
        print(f"Loaded {count} records into feed_coldstart")

    except Exception as e:
        conn.rollback()
        raise e
    finally:
        cur.close()
        conn.close()


if __name__ == "__main__":
    load_coldstart()
