// #455: Accessible tag input combobox with debounced, cancellable autocomplete.
import React, { useState, useEffect, useRef, useId } from "react";
import { autocompleteTags } from "../services/api";

function normalizeTagInput(raw) {
  return String(raw || "")
    .trim()
    .replace(/\s+/g, " ")
    .toLowerCase();
}

function TagInputCombobox({
  tags = [],
  onChange,
  label = "Tags",
  placeholder = "Add a tag…",
  disabled = false,
  id,
}) {
  const generatedId = useId();
  const inputId = id || `tag-combobox-${generatedId}`;
  const listboxId = `${inputId}-listbox`;

  const [inputValue, setInputValue] = useState("");
  const [suggestions, setSuggestions] = useState([]);
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const [announcement, setAnnouncement] = useState("");
  const abortRef = useRef(null);

  useEffect(() => {
    const trimmed = inputValue.trim();
    if (!trimmed) {
      if (abortRef.current) abortRef.current.abort();
      setSuggestions([]);
      setIsOpen(false);
      setActiveIndex(-1);
      return undefined;
    }

    const timer = setTimeout(async () => {
      if (abortRef.current) abortRef.current.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const res = await autocompleteTags(trimmed, 10, { signal: controller.signal });
        const items = (res.data?.tags || []).filter((item) => !tags.includes(item.tag));
        setSuggestions(items);
        setIsOpen(items.length > 0);
        setActiveIndex(-1);
        setAnnouncement(
          items.length === 0
            ? "No matching tags"
            : `${items.length} tag suggestion${items.length === 1 ? "" : "s"} available`
        );
      } catch (err) {
        if (err?.name !== "CanceledError" && err?.name !== "AbortError") {
          setSuggestions([]);
          setIsOpen(false);
        }
      }
    }, 250);

    return () => {
      clearTimeout(timer);
      if (abortRef.current) abortRef.current.abort();
    };
  }, [inputValue, tags]);

  const addTag = (raw) => {
    const nextTag = normalizeTagInput(raw);
    if (!nextTag || tags.includes(nextTag)) {
      setInputValue("");
      setIsOpen(false);
      setActiveIndex(-1);
      return;
    }
    const updated = [...tags, nextTag];
    onChange(updated);
    setAnnouncement(`Added tag ${nextTag}`);
    setInputValue("");
    setSuggestions([]);
    setIsOpen(false);
    setActiveIndex(-1);
  };

  const removeTag = (tagToRemove) => {
    const updated = tags.filter((t) => t !== tagToRemove);
    onChange(updated);
    setAnnouncement(`Removed tag ${tagToRemove}`);
  };

  const handleKeyDown = (event) => {
    if (event.key === "ArrowDown") {
      if (!isOpen && suggestions.length > 0) {
        event.preventDefault();
        setIsOpen(true);
        setActiveIndex(0);
      } else if (isOpen && suggestions.length > 0) {
        event.preventDefault();
        setActiveIndex((prev) => (prev + 1) % suggestions.length);
      }
    } else if (event.key === "ArrowUp") {
      if (isOpen && suggestions.length > 0) {
        event.preventDefault();
        setActiveIndex((prev) => (prev <= 0 ? suggestions.length - 1 : prev - 1));
      }
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (isOpen && activeIndex >= 0 && suggestions[activeIndex]) {
        addTag(suggestions[activeIndex].tag);
      } else if (inputValue.trim()) {
        addTag(inputValue);
      }
    } else if (event.key === ",") {
      event.preventDefault();
      if (inputValue.trim()) {
        addTag(inputValue);
      }
    } else if (event.key === "Escape") {
      if (isOpen) {
        event.preventDefault();
        setIsOpen(false);
        setActiveIndex(-1);
      }
    } else if (event.key === "Backspace" && inputValue === "" && tags.length > 0) {
      // Backspace on an empty input removes the most recently added tag.
      removeTag(tags[tags.length - 1]);
    }
  };

  const activeDescendantId =
    isOpen && activeIndex >= 0 && suggestions[activeIndex]
      ? `${listboxId}-opt-${activeIndex}`
      : undefined;

  return (
    <div className="tag-combobox-wrapper">
      <label htmlFor={inputId} className="tag-combobox-label">
        {label}
      </label>
      <div className="tag-chip-list">
        {tags.map((tag) => (
          <span key={tag} className="tag-chip">
            <span>{tag}</span>
            <button
              type="button"
              className="tag-chip-remove"
              onClick={() => removeTag(tag)}
              disabled={disabled}
              aria-label={`Remove tag ${tag}`}
            >
              ×
            </button>
          </span>
        ))}
      </div>
      <div className="tag-combobox-input-wrap">
        <input
          id={inputId}
          type="text"
          role="combobox"
          aria-expanded={isOpen}
          aria-controls={listboxId}
          aria-autocomplete="list"
          aria-activedescendant={activeDescendantId}
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          disabled={disabled}
          autoComplete="off"
        />
        {isOpen && suggestions.length > 0 && (
          <ul id={listboxId} role="listbox" className="tag-listbox" aria-label="Tag suggestions">
            {suggestions.map((item, idx) => (
              <li
                key={item.tag}
                id={`${listboxId}-opt-${idx}`}
                role="option"
                aria-selected={idx === activeIndex}
                className={`tag-option${idx === activeIndex ? " is-active" : ""}`}
                onMouseDown={(e) => {
                  e.preventDefault();
                  addTag(item.tag);
                }}
              >
                <span>{item.tag}</span>
                {typeof item.count === "number" && (
                  <span className="tag-option-count">{item.count}</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="sr-only" aria-live="polite">
        {announcement}
      </div>
    </div>
  );
}

export default TagInputCombobox;
