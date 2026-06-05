// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2025 Datadog, Inc.

package app;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.io.IOException;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetAddress;

@SpringBootApplication
@RestController
public class App {
    private static final Logger logger = LoggerFactory.getLogger(App.class);

    public static void main(String[] args) {
        SpringApplication.run(App.class, args);
    }

    @GetMapping("/")
    public String helloWorld() {
        logger.info("Hello logger using Java!");
        sendDistribution("java.example.requests", 1, "endpoint:root");
        return "Hello Java World!";
    }

    // Minimal DogStatsD client - the serverless-init sidecar listens on UDP 8125.
    // Serverless only supports the distribution metric type.
    private void sendDistribution(String metric, double value, String tags) {
        try (DatagramSocket socket = new DatagramSocket()) {
            byte[] payload = (metric + ":" + value + "|d|#" + tags).getBytes();
            DatagramPacket packet = new DatagramPacket(
                    payload, payload.length, InetAddress.getByName("127.0.0.1"), 8125);
            socket.send(packet);
        } catch (IOException e) {
            // best-effort metric emission
        }
    }
}
