# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2025 Datadog, Inc.

FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /app

COPY pom.xml ./
RUN mvn dependency:go-offline -q

COPY . .
RUN mvn package -q -DskipTests

FROM eclipse-temurin:21-jre
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

# Install Datadog tracer (pinned for reproducible builds)
ARG DD_TRACER_VERSION=1.63.0
RUN curl -fsSL -o agent.jar \
    "https://repo1.maven.org/maven2/com/datadoghq/dd-java-agent/${DD_TRACER_VERSION}/dd-java-agent-${DD_TRACER_VERSION}.jar"
COPY --from=build /app/target/*.jar app.jar

ENV PORT=8080
EXPOSE 8080

# Datadog automatic instrumentation environment variable
ENV JAVA_TOOL_OPTIONS="-javaagent:/app/agent.jar"

CMD ["java", "-jar", "app.jar"]
