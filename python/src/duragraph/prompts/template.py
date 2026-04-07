"""Prompt template rendering with variable substitution and versioning."""

import re
from dataclasses import dataclass, field
from typing import Any


@dataclass
class PromptTemplate:
    """A prompt template with variable substitution support.

    Variables use {{variable_name}} syntax. Templates can have
    metadata for versioning and A/B testing.
    """

    template: str
    name: str
    version: str = "1.0"
    variant: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    _VAR_PATTERN = re.compile(r"\{\{(\w+)\}\}")

    def render(self, **variables: Any) -> str:
        """Render the template with the given variables.

        Args:
            **variables: Key-value pairs to substitute into the template.

        Returns:
            The rendered string.

        Raises:
            ValueError: If required variables are missing.
        """
        required = self.variables
        missing = required - set(variables.keys())
        if missing:
            raise ValueError(f"Missing template variables: {', '.join(sorted(missing))}")
        result = self.template
        for key, value in variables.items():
            result = result.replace(f"{{{{{key}}}}}", str(value))
        return result

    @property
    def variables(self) -> set[str]:
        """Return the set of variable names used in the template."""
        return set(self._VAR_PATTERN.findall(self.template))

    def with_version(self, version: str) -> "PromptTemplate":
        """Return a copy of this template with a different version."""
        return PromptTemplate(
            template=self.template,
            name=self.name,
            version=version,
            variant=self.variant,
            metadata=self.metadata.copy(),
        )

    def with_variant(self, variant: str) -> "PromptTemplate":
        """Return a copy of this template with a different variant."""
        return PromptTemplate(
            template=self.template,
            name=self.name,
            version=self.version,
            variant=variant,
            metadata=self.metadata.copy(),
        )


class PromptLibrary:
    """In-memory prompt library with versioning and caching.

    Use for local development and testing. For production, use
    PromptStore which connects to the control plane.
    """

    def __init__(self) -> None:
        self._templates: dict[str, dict[str, PromptTemplate]] = {}

    def register(self, template: PromptTemplate) -> None:
        """Register a prompt template."""
        key = template.name
        if key not in self._templates:
            self._templates[key] = {}
        self._templates[key][template.version] = template

    def get(
        self,
        name: str,
        *,
        version: str | None = None,
    ) -> PromptTemplate:
        """Get a prompt template by name and optional version.

        Args:
            name: Template name.
            version: Specific version. If None, returns the latest.

        Returns:
            The matching PromptTemplate.

        Raises:
            KeyError: If template or version not found.
        """
        versions = self._templates.get(name)
        if not versions:
            raise KeyError(f"Prompt template '{name}' not found")
        if version is not None:
            if version not in versions:
                raise KeyError(f"Version '{version}' not found for template '{name}'")
            return versions[version]
        latest_version = sorted(versions.keys())[-1]
        return versions[latest_version]

    def list_templates(self) -> list[str]:
        """Return all registered template names."""
        return list(self._templates.keys())

    def list_versions(self, name: str) -> list[str]:
        """Return all versions for a template."""
        versions = self._templates.get(name)
        if not versions:
            raise KeyError(f"Prompt template '{name}' not found")
        return sorted(versions.keys())
