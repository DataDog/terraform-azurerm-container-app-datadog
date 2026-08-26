# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

from flask import Flask

app = Flask(__name__)


@app.get("/")
def hello_world():
    return "Hello Python SSI World!"


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
