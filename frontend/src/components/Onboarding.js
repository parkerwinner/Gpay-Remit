import React, { useState, useEffect, useCallback } from "react";
import "./Onboarding.css";

const ONBOARDING_STEPS = [
  {
    title: "Welcome to Gpay-Remit",
    badge: "Getting Started",
    description:
      "Gpay-Remit is a next-generation cross-border payment platform powered by the Stellar network. Send funds globally in seconds with ultra-low fees, real-time FX rates, and bank-grade security.",
    icon: "🌐",
    highlights: [
      "Sub-second global settlements via Stellar",
      "Transparent fees with real-time currency conversion",
      "Enterprise security with AES-256 data encryption at rest",
    ],
  },
  {
    title: "Fast Cross-Border Remittances",
    badge: "Send Money",
    description:
      "Initiate instant remittances by selecting your source and target currencies, entering the amount, and providing the recipient's Stellar address.",
    icon: "💸",
    highlights: [
      "Dynamic fee estimation and live forex conversion rates",
      "Smart contract escrow protection for guaranteed delivery",
      "Instant recipient notification upon completion",
    ],
  },
  {
    title: "Escrow Invoices & Requests",
    badge: "Invoices",
    description:
      "Generate, manage, and settle blockchain-verified payment requests. Escrow smart contracts securely hold funds until condition milestones are met.",
    icon: "📄",
    highlights: [
      "Create and send payment requests in multiple currencies",
      "Real-time invoice status tracking and QR code payments",
      "Cryptographic dispute resolution and release triggers",
    ],
  },
  {
    title: "Live History & Enterprise SLA",
    badge: "Transactions",
    description:
      "Monitor all your transfers in real-time with comprehensive transaction logs, audit trails, and 99.9% uptime SLA performance metrics.",
    icon: "📊",
    highlights: [
      "Direct links to Stellar blockchain transaction explorers",
      "Downloadable PDF and CSV transaction receipts",
      "Sub-500ms P95 latency tracking and operational metrics",
    ],
  },
  {
    title: "Security & Account Settings",
    badge: "Security",
    description:
      "Your sensitive data (SSN, banking details) is automatically encrypted at rest. Protect your account with Two-Factor Authentication (2FA) and custom notifications.",
    icon: "🛡️",
    highlights: [
      "Automated AES-256-GCM application-level encryption",
      "TOTP authenticator app support (Google Authenticator, Authy)",
      "Granular email and webhook notification preferences",
    ],
  },
];

const STORAGE_KEY = "gpay_onboarding_completed";

export function Onboarding({ isOpen, onClose, onComplete }) {
  const [currentStep, setCurrentStep] = useState(0);

  const handleClose = useCallback(() => {
    localStorage.setItem(STORAGE_KEY, "true");
    if (onClose) onClose();
  }, [onClose]);

  const handleFinish = useCallback(() => {
    localStorage.setItem(STORAGE_KEY, "true");
    if (onComplete) onComplete();
    if (onClose) onClose();
  }, [onClose, onComplete]);

  const handleNext = useCallback(() => {
    if (currentStep < ONBOARDING_STEPS.length - 1) {
      setCurrentStep((prev) => prev + 1);
    } else {
      handleFinish();
    }
  }, [currentStep, handleFinish]);

  const handlePrev = useCallback(() => {
    if (currentStep > 0) {
      setCurrentStep((prev) => prev - 1);
    }
  }, [currentStep]);

  // Keyboard navigation
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e) => {
      if (e.key === "Escape") {
        handleClose();
      } else if (e.key === "ArrowRight") {
        handleNext();
      } else if (e.key === "ArrowLeft") {
        handlePrev();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, handleClose, handleNext, handlePrev]);

  if (!isOpen) return null;

  const step = ONBOARDING_STEPS[currentStep];
  const isLastStep = currentStep === ONBOARDING_STEPS.length - 1;

  return (
    <div className="onboarding-overlay" role="dialog" aria-modal="true" aria-labelledby="onboarding-title">
      <div className="onboarding-modal">
        <div className="onboarding-header">
          <span className="onboarding-badge">{step.badge}</span>
          <button
            type="button"
            className="onboarding-close-btn"
            onClick={handleClose}
            aria-label="Close onboarding tour"
          >
            &times;
          </button>
        </div>

        <div className="onboarding-body">
          <div className="onboarding-icon" aria-hidden="true">
            {step.icon}
          </div>
          <h2 id="onboarding-title" className="onboarding-title">
            {step.title}
          </h2>
          <p className="onboarding-description">{step.description}</p>

          <div className="onboarding-highlights">
            {step.highlights.map((highlight, idx) => (
              <div key={idx} className="onboarding-highlight-item">
                <span className="highlight-check">✓</span>
                <span>{highlight}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="onboarding-footer">
          <div className="onboarding-progress" aria-label={`Step ${currentStep + 1} of ${ONBOARDING_STEPS.length}`}>
            {ONBOARDING_STEPS.map((_, idx) => (
              <button
                key={idx}
                type="button"
                className={`progress-dot ${idx === currentStep ? "active" : ""} ${idx < currentStep ? "completed" : ""}`}
                onClick={() => setCurrentStep(idx)}
                aria-label={`Go to step ${idx + 1}`}
              />
            ))}
          </div>

          <div className="onboarding-actions">
            <button
              type="button"
              className="onboarding-btn onboarding-btn-skip"
              onClick={handleClose}
            >
              Skip Tour
            </button>

            {currentStep > 0 && (
              <button
                type="button"
                className="onboarding-btn onboarding-btn-secondary"
                onClick={handlePrev}
              >
                Back
              </button>
            )}

            <button
              type="button"
              className="onboarding-btn onboarding-btn-primary"
              onClick={handleNext}
            >
              {isLastStep ? "Get Started" : "Next →"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default Onboarding;
