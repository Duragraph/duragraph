"""Tests for prompt template and library."""

from duragraph.prompts.template import PromptLibrary, PromptTemplate


def test_prompt_template_render():
    tmpl = PromptTemplate(
        template="Hello {{name}}, you are a {{role}}.",
        name="greeting",
    )
    result = tmpl.render(name="Alice", role="engineer")
    assert result == "Hello Alice, you are a engineer."


def test_prompt_template_variables():
    tmpl = PromptTemplate(
        template="{{a}} and {{b}} and {{a}}",
        name="dup",
    )
    assert tmpl.variables == {"a", "b"}


def test_prompt_template_missing_variable():
    tmpl = PromptTemplate(
        template="Hello {{name}}",
        name="greet",
    )
    try:
        tmpl.render()
        raise AssertionError("Should have raised ValueError")
    except ValueError as e:
        assert "name" in str(e)


def test_prompt_template_with_version():
    tmpl = PromptTemplate(template="v1", name="t", version="1.0")
    v2 = tmpl.with_version("2.0")
    assert v2.version == "2.0"
    assert v2.template == "v1"
    assert tmpl.version == "1.0"


def test_prompt_template_with_variant():
    tmpl = PromptTemplate(template="base", name="t")
    ab = tmpl.with_variant("treatment")
    assert ab.variant == "treatment"
    assert tmpl.variant is None


def test_prompt_library_register_and_get():
    lib = PromptLibrary()
    tmpl = PromptTemplate(template="Hello {{name}}", name="greet", version="1.0")
    lib.register(tmpl)
    got = lib.get("greet")
    assert got.template == "Hello {{name}}"


def test_prompt_library_latest_version():
    lib = PromptLibrary()
    lib.register(PromptTemplate(template="v1", name="t", version="1.0"))
    lib.register(PromptTemplate(template="v2", name="t", version="2.0"))
    latest = lib.get("t")
    assert latest.template == "v2"


def test_prompt_library_specific_version():
    lib = PromptLibrary()
    lib.register(PromptTemplate(template="v1", name="t", version="1.0"))
    lib.register(PromptTemplate(template="v2", name="t", version="2.0"))
    got = lib.get("t", version="1.0")
    assert got.template == "v1"


def test_prompt_library_not_found():
    lib = PromptLibrary()
    try:
        lib.get("missing")
        raise AssertionError("Should have raised KeyError")
    except KeyError:
        pass


def test_prompt_library_list_templates():
    lib = PromptLibrary()
    lib.register(PromptTemplate(template="a", name="alpha"))
    lib.register(PromptTemplate(template="b", name="beta"))
    assert sorted(lib.list_templates()) == ["alpha", "beta"]


def test_prompt_library_list_versions():
    lib = PromptLibrary()
    lib.register(PromptTemplate(template="v1", name="t", version="1.0"))
    lib.register(PromptTemplate(template="v2", name="t", version="2.0"))
    assert lib.list_versions("t") == ["1.0", "2.0"]


def test_prompt_store_caching():
    """Test PromptStore cache invalidation."""
    from duragraph.prompts.store import PromptStore, _CacheEntry

    entry = _CacheEntry({"content": "hi"}, ttl=300)
    assert not entry.expired
    assert entry.value == {"content": "hi"}

    store = PromptStore("http://localhost:8080", cache_ttl=0)
    assert store._get_cached("any") is None

    store2 = PromptStore("http://localhost:8080", cache_ttl=300)
    store2._set_cached("prompt:latest:default", {"content": "cached"})
    assert store2._get_cached("prompt:latest:default") == {"content": "cached"}

    store2.invalidate("prompt")
    assert store2._get_cached("prompt:latest:default") is None


def test_prompt_store_invalidate_all():
    from duragraph.prompts.store import PromptStore

    store = PromptStore("http://localhost:8080", cache_ttl=300)
    store._set_cached("a:latest:default", {"content": "a"})
    store._set_cached("b:latest:default", {"content": "b"})
    store.invalidate()
    assert store._get_cached("a:latest:default") is None
    assert store._get_cached("b:latest:default") is None
