"""Bounded OCR failures cannot supply unsafe coordinates to device actions."""
import subprocess
import unittest
from unittest.mock import patch
from PIL import Image
from tools.device_demo import Device


class DeviceOCRTests(unittest.TestCase):
    def setUp(self):
        self.device = Device("test-device")
        self.frame = Image.new("RGB", (1280, 576), "white")

    def recognize(self, response):
        with patch.object(self.device, "frame", return_value=self.frame), \
                patch("tools.device_demo.subprocess.run", return_value=response):
            return self.device.ocr("招募")

    def test_native_deadline_returns_manual_observation_error(self):
        response = subprocess.CompletedProcess([], 124, "", "Local screenshot OCR timed out")
        with self.assertRaisesRegex(RuntimeError, "timed out; inspect"):
            self.recognize(response)

    def test_process_timeout_does_not_expose_arguments(self):
        with patch.object(self.device, "frame", return_value=self.frame), \
                patch("tools.device_demo.subprocess.run", side_effect=subprocess.TimeoutExpired(["private-filter"], 15)):
            with self.assertRaises(RuntimeError) as caught:
                self.device.ocr("招募")
        self.assertNotIn("private-filter", str(caught.exception))
        self.assertTrue(caught.exception.__suppress_context__)

    def test_malformed_or_outside_coordinates_are_rejected(self):
        for payload in ["not-json", "{}", '[{"text":"招募","confidence":1,"x":2,"y":0.5,"width":0.1,"height":0.1}]']:
            with self.subTest(payload=payload):
                with self.assertRaisesRegex(RuntimeError, "invalid coordinates"):
                    self.recognize(subprocess.CompletedProcess([], 0, payload, ""))

    def test_normalized_results_remain_available(self):
        payload = '[{"text":"招募","confidence":1,"x":0.4,"y":0.8,"width":0.1,"height":0.1}]'
        result = self.recognize(subprocess.CompletedProcess([], 0, payload, ""))
        self.assertEqual(result[0]["text"], "招募")
        self.assertEqual(result[0]["x"], 0.4)


if __name__ == "__main__":
    unittest.main()
