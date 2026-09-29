/*! ketchup a5c429dbc4990ae0f42320a5bfc6cfd3045cba21 (claude/ketchup-plugin-u5j6e6) from https://github.com/egeozcan/ketchup.git, built by scripts/update-ketchup.sh with node v22.22.2 */
/**
 * @license
 * Copyright 2019 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const se=globalThis,Le=se.ShadowRoot&&(se.ShadyCSS===void 0||se.ShadyCSS.nativeShadow)&&"adoptedStyleSheets"in Document.prototype&&"replace"in CSSStyleSheet.prototype,ze=Symbol(),qe=new WeakMap;let Is=class{constructor(e,s,i){if(this._$cssResult$=!0,i!==ze)throw Error("CSSResult is not constructable. Use `unsafeCSS` or `css` instead.");this.cssText=e,this.t=s}get styleSheet(){let e=this.o;const s=this.t;if(Le&&e===void 0){const i=s!==void 0&&s.length===1;i&&(e=qe.get(s)),e===void 0&&((this.o=e=new CSSStyleSheet).replaceSync(this.cssText),i&&qe.set(s,e))}return e}toString(){return this.cssText}};const Qs=t=>new Is(typeof t=="string"?t:t+"",void 0,ze),bt=(t,...e)=>{const s=t.length===1?t[0]:e.reduce((i,a,o)=>i+(r=>{if(r._$cssResult$===!0)return r.cssText;if(typeof r=="number")return r;throw Error("Value passed to 'css' function must be a 'css' function result: "+r+". Use 'unsafeCSS' to pass non-literal values, but take care to ensure page security.")})(a)+t[o+1],t[0]);return new Is(s,t,ze)},ti=(t,e)=>{if(Le)t.adoptedStyleSheets=e.map(s=>s instanceof CSSStyleSheet?s:s.styleSheet);else for(const s of e){const i=document.createElement("style"),a=se.litNonce;a!==void 0&&i.setAttribute("nonce",a),i.textContent=s.cssText,t.appendChild(i)}},Ze=Le?t=>t:t=>t instanceof CSSStyleSheet?(e=>{let s="";for(const i of e.cssRules)s+=i.cssText;return Qs(s)})(t):t;/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const{is:ei,defineProperty:si,getOwnPropertyDescriptor:ii,getOwnPropertyNames:ai,getOwnPropertySymbols:oi,getPrototypeOf:ri}=Object,pe=globalThis,Ke=pe.trustedTypes,ni=Ke?Ke.emptyScript:"",ci=pe.reactiveElementPolyfillSupport,Bt=(t,e)=>t,ne={toAttribute(t,e){switch(e){case Boolean:t=t?ni:null;break;case Object:case Array:t=t==null?t:JSON.stringify(t)}return t},fromAttribute(t,e){let s=t;switch(e){case Boolean:s=t!==null;break;case Number:s=t===null?null:Number(t);break;case Object:case Array:try{s=JSON.parse(t)}catch{s=null}}return s}},Ae=(t,e)=>!ei(t,e),Ge={attribute:!0,type:String,converter:ne,reflect:!1,useDefault:!1,hasChanged:Ae};Symbol.metadata??=Symbol("metadata"),pe.litPropertyMetadata??=new WeakMap;let $t=class extends HTMLElement{static addInitializer(e){this._$Ei(),(this.l??=[]).push(e)}static get observedAttributes(){return this.finalize(),this._$Eh&&[...this._$Eh.keys()]}static createProperty(e,s=Ge){if(s.state&&(s.attribute=!1),this._$Ei(),this.prototype.hasOwnProperty(e)&&((s=Object.create(s)).wrapped=!0),this.elementProperties.set(e,s),!s.noAccessor){const i=Symbol(),a=this.getPropertyDescriptor(e,i,s);a!==void 0&&si(this.prototype,e,a)}}static getPropertyDescriptor(e,s,i){const{get:a,set:o}=ii(this.prototype,e)??{get(){return this[s]},set(r){this[s]=r}};return{get:a,set(r){const n=a?.call(this);o?.call(this,r),this.requestUpdate(e,n,i)},configurable:!0,enumerable:!0}}static getPropertyOptions(e){return this.elementProperties.get(e)??Ge}static _$Ei(){if(this.hasOwnProperty(Bt("elementProperties")))return;const e=ri(this);e.finalize(),e.l!==void 0&&(this.l=[...e.l]),this.elementProperties=new Map(e.elementProperties)}static finalize(){if(this.hasOwnProperty(Bt("finalized")))return;if(this.finalized=!0,this._$Ei(),this.hasOwnProperty(Bt("properties"))){const s=this.properties,i=[...ai(s),...oi(s)];for(const a of i)this.createProperty(a,s[a])}const e=this[Symbol.metadata];if(e!==null){const s=litPropertyMetadata.get(e);if(s!==void 0)for(const[i,a]of s)this.elementProperties.set(i,a)}this._$Eh=new Map;for(const[s,i]of this.elementProperties){const a=this._$Eu(s,i);a!==void 0&&this._$Eh.set(a,s)}this.elementStyles=this.finalizeStyles(this.styles)}static finalizeStyles(e){const s=[];if(Array.isArray(e)){const i=new Set(e.flat(1/0).reverse());for(const a of i)s.unshift(Ze(a))}else e!==void 0&&s.push(Ze(e));return s}static _$Eu(e,s){const i=s.attribute;return i===!1?void 0:typeof i=="string"?i:typeof e=="string"?e.toLowerCase():void 0}constructor(){super(),this._$Ep=void 0,this.isUpdatePending=!1,this.hasUpdated=!1,this._$Em=null,this._$Ev()}_$Ev(){this._$ES=new Promise(e=>this.enableUpdating=e),this._$AL=new Map,this._$E_(),this.requestUpdate(),this.constructor.l?.forEach(e=>e(this))}addController(e){(this._$EO??=new Set).add(e),this.renderRoot!==void 0&&this.isConnected&&e.hostConnected?.()}removeController(e){this._$EO?.delete(e)}_$E_(){const e=new Map,s=this.constructor.elementProperties;for(const i of s.keys())this.hasOwnProperty(i)&&(e.set(i,this[i]),delete this[i]);e.size>0&&(this._$Ep=e)}createRenderRoot(){const e=this.shadowRoot??this.attachShadow(this.constructor.shadowRootOptions);return ti(e,this.constructor.elementStyles),e}connectedCallback(){this.renderRoot??=this.createRenderRoot(),this.enableUpdating(!0),this._$EO?.forEach(e=>e.hostConnected?.())}enableUpdating(e){}disconnectedCallback(){this._$EO?.forEach(e=>e.hostDisconnected?.())}attributeChangedCallback(e,s,i){this._$AK(e,i)}_$ET(e,s){const i=this.constructor.elementProperties.get(e),a=this.constructor._$Eu(e,i);if(a!==void 0&&i.reflect===!0){const o=(i.converter?.toAttribute!==void 0?i.converter:ne).toAttribute(s,i.type);this._$Em=e,o==null?this.removeAttribute(a):this.setAttribute(a,o),this._$Em=null}}_$AK(e,s){const i=this.constructor,a=i._$Eh.get(e);if(a!==void 0&&this._$Em!==a){const o=i.getPropertyOptions(a),r=typeof o.converter=="function"?{fromAttribute:o.converter}:o.converter?.fromAttribute!==void 0?o.converter:ne;this._$Em=a;const n=r.fromAttribute(s,o.type);this[a]=n??this._$Ej?.get(a)??n,this._$Em=null}}requestUpdate(e,s,i,a=!1,o){if(e!==void 0){const r=this.constructor;if(a===!1&&(o=this[e]),i??=r.getPropertyOptions(e),!((i.hasChanged??Ae)(o,s)||i.useDefault&&i.reflect&&o===this._$Ej?.get(e)&&!this.hasAttribute(r._$Eu(e,i))))return;this.C(e,s,i)}this.isUpdatePending===!1&&(this._$ES=this._$EP())}C(e,s,{useDefault:i,reflect:a,wrapped:o},r){i&&!(this._$Ej??=new Map).has(e)&&(this._$Ej.set(e,r??s??this[e]),o!==!0||r!==void 0)||(this._$AL.has(e)||(this.hasUpdated||i||(s=void 0),this._$AL.set(e,s)),a===!0&&this._$Em!==e&&(this._$Eq??=new Set).add(e))}async _$EP(){this.isUpdatePending=!0;try{await this._$ES}catch(s){Promise.reject(s)}const e=this.scheduleUpdate();return e!=null&&await e,!this.isUpdatePending}scheduleUpdate(){return this.performUpdate()}performUpdate(){if(!this.isUpdatePending)return;if(!this.hasUpdated){if(this.renderRoot??=this.createRenderRoot(),this._$Ep){for(const[a,o]of this._$Ep)this[a]=o;this._$Ep=void 0}const i=this.constructor.elementProperties;if(i.size>0)for(const[a,o]of i){const{wrapped:r}=o,n=this[a];r!==!0||this._$AL.has(a)||n===void 0||this.C(a,void 0,o,n)}}let e=!1;const s=this._$AL;try{e=this.shouldUpdate(s),e?(this.willUpdate(s),this._$EO?.forEach(i=>i.hostUpdate?.()),this.update(s)):this._$EM()}catch(i){throw e=!1,this._$EM(),i}e&&this._$AE(s)}willUpdate(e){}_$AE(e){this._$EO?.forEach(s=>s.hostUpdated?.()),this.hasUpdated||(this.hasUpdated=!0,this.firstUpdated(e)),this.updated(e)}_$EM(){this._$AL=new Map,this.isUpdatePending=!1}get updateComplete(){return this.getUpdateComplete()}getUpdateComplete(){return this._$ES}shouldUpdate(e){return!0}update(e){this._$Eq&&=this._$Eq.forEach(s=>this._$ET(s,this[s])),this._$EM()}updated(e){}firstUpdated(e){}};$t.elementStyles=[],$t.shadowRootOptions={mode:"open"},$t[Bt("elementProperties")]=new Map,$t[Bt("finalized")]=new Map,ci?.({ReactiveElement:$t}),(pe.reactiveElementVersions??=[]).push("2.1.2");/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const Oe=globalThis,Je=t=>t,ce=Oe.trustedTypes,Qe=ce?ce.createPolicy("lit-html",{createHTML:t=>t}):void 0,Ds="$lit$",ct=`lit$${Math.random().toFixed(9).slice(2)}$`,Ts="?"+ct,li=`<${Ts}>`,vt=document,Xt=()=>vt.createComment(""),Ut=t=>t===null||typeof t!="object"&&typeof t!="function",je=Array.isArray,hi=t=>je(t)||typeof t?.[Symbol.iterator]=="function",be=`[ 	
\f\r]`,Ot=/<(?:(!--|\/[^a-zA-Z])|(\/?[a-zA-Z][^>\s]*)|(\/?$))/g,ts=/-->/g,es=/>/g,pt=RegExp(`>|${be}(?:([^\\s"'>=/]+)(${be}*=${be}*(?:[^ 	
\f\r"'\`<>=]|("|')|))|$)`,"g"),ss=/'/g,is=/"/g,Rs=/^(?:script|style|textarea|title)$/i,Es=t=>(e,...s)=>({_$litType$:t,strings:e,values:s}),g=Es(1),$=Es(2),Tt=Symbol.for("lit-noChange"),P=Symbol.for("lit-nothing"),as=new WeakMap,mt=vt.createTreeWalker(vt,129);function Ls(t,e){if(!je(t)||!t.hasOwnProperty("raw"))throw Error("invalid template strings array");return Qe!==void 0?Qe.createHTML(e):e}const di=(t,e)=>{const s=t.length-1,i=[];let a,o=e===2?"<svg>":e===3?"<math>":"",r=Ot;for(let n=0;n<s;n++){const c=t[n];let h,l,d=-1,f=0;for(;f<c.length&&(r.lastIndex=f,l=r.exec(c),l!==null);)f=r.lastIndex,r===Ot?l[1]==="!--"?r=ts:l[1]!==void 0?r=es:l[2]!==void 0?(Rs.test(l[2])&&(a=RegExp("</"+l[2],"g")),r=pt):l[3]!==void 0&&(r=pt):r===pt?l[0]===">"?(r=a??Ot,d=-1):l[1]===void 0?d=-2:(d=r.lastIndex-l[2].length,h=l[1],r=l[3]===void 0?pt:l[3]==='"'?is:ss):r===is||r===ss?r=pt:r===ts||r===es?r=Ot:(r=pt,a=void 0);const u=r===pt&&t[n+1].startsWith("/>")?" ":"";o+=r===Ot?c+li:d>=0?(i.push(h),c.slice(0,d)+Ds+c.slice(d)+ct+u):c+ct+(d===-2?n:u)}return[Ls(t,o+(t[s]||"<?>")+(e===2?"</svg>":e===3?"</math>":"")),i]};class Nt{constructor({strings:e,_$litType$:s},i){let a;this.parts=[];let o=0,r=0;const n=e.length-1,c=this.parts,[h,l]=di(e,s);if(this.el=Nt.createElement(h,i),mt.currentNode=this.el.content,s===2||s===3){const d=this.el.content.firstChild;d.replaceWith(...d.childNodes)}for(;(a=mt.nextNode())!==null&&c.length<n;){if(a.nodeType===1){if(a.hasAttributes())for(const d of a.getAttributeNames())if(d.endsWith(Ds)){const f=l[r++],u=a.getAttribute(d).split(ct),p=/([.?@])?(.*)/.exec(f);c.push({type:1,index:o,name:p[2],strings:u,ctor:p[1]==="."?ui:p[1]==="?"?fi:p[1]==="@"?_i:ue}),a.removeAttribute(d)}else d.startsWith(ct)&&(c.push({type:6,index:o}),a.removeAttribute(d));if(Rs.test(a.tagName)){const d=a.textContent.split(ct),f=d.length-1;if(f>0){a.textContent=ce?ce.emptyScript:"";for(let u=0;u<f;u++)a.append(d[u],Xt()),mt.nextNode(),c.push({type:2,index:++o});a.append(d[f],Xt())}}}else if(a.nodeType===8)if(a.data===Ts)c.push({type:2,index:o});else{let d=-1;for(;(d=a.data.indexOf(ct,d+1))!==-1;)c.push({type:7,index:o}),d+=ct.length-1}o++}}static createElement(e,s){const i=vt.createElement("template");return i.innerHTML=e,i}}function Rt(t,e,s=t,i){if(e===Tt)return e;let a=i!==void 0?s._$Co?.[i]:s._$Cl;const o=Ut(e)?void 0:e._$litDirective$;return a?.constructor!==o&&(a?._$AO?.(!1),o===void 0?a=void 0:(a=new o(t),a._$AT(t,s,i)),i!==void 0?(s._$Co??=[])[i]=a:s._$Cl=a),a!==void 0&&(e=Rt(t,a._$AS(t,e.values),a,i)),e}class pi{constructor(e,s){this._$AV=[],this._$AN=void 0,this._$AD=e,this._$AM=s}get parentNode(){return this._$AM.parentNode}get _$AU(){return this._$AM._$AU}u(e){const{el:{content:s},parts:i}=this._$AD,a=(e?.creationScope??vt).importNode(s,!0);mt.currentNode=a;let o=mt.nextNode(),r=0,n=0,c=i[0];for(;c!==void 0;){if(r===c.index){let h;c.type===2?h=new Vt(o,o.nextSibling,this,e):c.type===1?h=new c.ctor(o,c.name,c.strings,this,e):c.type===6&&(h=new mi(o,this,e)),this._$AV.push(h),c=i[++n]}r!==c?.index&&(o=mt.nextNode(),r++)}return mt.currentNode=vt,a}p(e){let s=0;for(const i of this._$AV)i!==void 0&&(i.strings!==void 0?(i._$AI(e,i,s),s+=i.strings.length-2):i._$AI(e[s])),s++}}class Vt{get _$AU(){return this._$AM?._$AU??this._$Cv}constructor(e,s,i,a){this.type=2,this._$AH=P,this._$AN=void 0,this._$AA=e,this._$AB=s,this._$AM=i,this.options=a,this._$Cv=a?.isConnected??!0}get parentNode(){let e=this._$AA.parentNode;const s=this._$AM;return s!==void 0&&e?.nodeType===11&&(e=s.parentNode),e}get startNode(){return this._$AA}get endNode(){return this._$AB}_$AI(e,s=this){e=Rt(this,e,s),Ut(e)?e===P||e==null||e===""?(this._$AH!==P&&this._$AR(),this._$AH=P):e!==this._$AH&&e!==Tt&&this._(e):e._$litType$!==void 0?this.$(e):e.nodeType!==void 0?this.T(e):hi(e)?this.k(e):this._(e)}O(e){return this._$AA.parentNode.insertBefore(e,this._$AB)}T(e){this._$AH!==e&&(this._$AR(),this._$AH=this.O(e))}_(e){this._$AH!==P&&Ut(this._$AH)?this._$AA.nextSibling.data=e:this.T(vt.createTextNode(e)),this._$AH=e}$(e){const{values:s,_$litType$:i}=e,a=typeof i=="number"?this._$AC(e):(i.el===void 0&&(i.el=Nt.createElement(Ls(i.h,i.h[0]),this.options)),i);if(this._$AH?._$AD===a)this._$AH.p(s);else{const o=new pi(a,this),r=o.u(this.options);o.p(s),this.T(r),this._$AH=o}}_$AC(e){let s=as.get(e.strings);return s===void 0&&as.set(e.strings,s=new Nt(e)),s}k(e){je(this._$AH)||(this._$AH=[],this._$AR());const s=this._$AH;let i,a=0;for(const o of e)a===s.length?s.push(i=new Vt(this.O(Xt()),this.O(Xt()),this,this.options)):i=s[a],i._$AI(o),a++;a<s.length&&(this._$AR(i&&i._$AB.nextSibling,a),s.length=a)}_$AR(e=this._$AA.nextSibling,s){for(this._$AP?.(!1,!0,s);e!==this._$AB;){const i=Je(e).nextSibling;Je(e).remove(),e=i}}setConnected(e){this._$AM===void 0&&(this._$Cv=e,this._$AP?.(e))}}class ue{get tagName(){return this.element.tagName}get _$AU(){return this._$AM._$AU}constructor(e,s,i,a,o){this.type=1,this._$AH=P,this._$AN=void 0,this.element=e,this.name=s,this._$AM=a,this.options=o,i.length>2||i[0]!==""||i[1]!==""?(this._$AH=Array(i.length-1).fill(new String),this.strings=i):this._$AH=P}_$AI(e,s=this,i,a){const o=this.strings;let r=!1;if(o===void 0)e=Rt(this,e,s,0),r=!Ut(e)||e!==this._$AH&&e!==Tt,r&&(this._$AH=e);else{const n=e;let c,h;for(e=o[0],c=0;c<o.length-1;c++)h=Rt(this,n[i+c],s,c),h===Tt&&(h=this._$AH[c]),r||=!Ut(h)||h!==this._$AH[c],h===P?e=P:e!==P&&(e+=(h??"")+o[c+1]),this._$AH[c]=h}r&&!a&&this.j(e)}j(e){e===P?this.element.removeAttribute(this.name):this.element.setAttribute(this.name,e??"")}}class ui extends ue{constructor(){super(...arguments),this.type=3}j(e){this.element[this.name]=e===P?void 0:e}}class fi extends ue{constructor(){super(...arguments),this.type=4}j(e){this.element.toggleAttribute(this.name,!!e&&e!==P)}}class _i extends ue{constructor(e,s,i,a,o){super(e,s,i,a,o),this.type=5}_$AI(e,s=this){if((e=Rt(this,e,s,0)??P)===Tt)return;const i=this._$AH,a=e===P&&i!==P||e.capture!==i.capture||e.once!==i.once||e.passive!==i.passive,o=e!==P&&(i===P||a);a&&this.element.removeEventListener(this.name,this,i),o&&this.element.addEventListener(this.name,this,e),this._$AH=e}handleEvent(e){typeof this._$AH=="function"?this._$AH.call(this.options?.host??this.element,e):this._$AH.handleEvent(e)}}class mi{constructor(e,s,i){this.element=e,this.type=6,this._$AN=void 0,this._$AM=s,this.options=i}get _$AU(){return this._$AM._$AU}_$AI(e){Rt(this,e)}}const gi=Oe.litHtmlPolyfillSupport;gi?.(Nt,Vt),(Oe.litHtmlVersions??=[]).push("3.3.2");const vi=(t,e,s)=>{const i=s?.renderBefore??e;let a=i._$litPart$;if(a===void 0){const o=s?.renderBefore??null;i._$litPart$=a=new Vt(e.insertBefore(Xt(),o),o,void 0,s??{})}return a._$AI(t),a};/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const He=globalThis;let F=class extends $t{constructor(){super(...arguments),this.renderOptions={host:this},this._$Do=void 0}createRenderRoot(){const e=super.createRenderRoot();return this.renderOptions.renderBefore??=e.firstChild,e}update(e){const s=this.render();this.hasUpdated||(this.renderOptions.isConnected=this.isConnected),super.update(e),this._$Do=vi(s,this.renderRoot,this.renderOptions)}connectedCallback(){super.connectedCallback(),this._$Do?.setConnected(!0)}disconnectedCallback(){super.disconnectedCallback(),this._$Do?.setConnected(!1)}render(){return Tt}};F._$litElement$=!0,F.finalized=!0,He.litElementHydrateSupport?.({LitElement:F});const bi=He.litElementPolyfillSupport;bi?.({LitElement:F});(He.litElementVersions??=[]).push("4.2.2");/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const yt=t=>(e,s)=>{s!==void 0?s.addInitializer(()=>{customElements.define(t,e)}):customElements.define(t,e)};/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const yi={attribute:!0,type:String,converter:ne,reflect:!1,hasChanged:Ae},wi=(t=yi,e,s)=>{const{kind:i,metadata:a}=s;let o=globalThis.litPropertyMetadata.get(a);if(o===void 0&&globalThis.litPropertyMetadata.set(a,o=new Map),i==="setter"&&((t=Object.create(t)).wrapped=!0),o.set(s.name,t),i==="accessor"){const{name:r}=s;return{set(n){const c=e.get.call(this);e.set.call(this,n),this.requestUpdate(r,c,t,!0,n)},init(n){return n!==void 0&&this.C(r,void 0,t,n),n}}}if(i==="setter"){const{name:r}=s;return function(n){const c=this[r];e.call(this,n),this.requestUpdate(r,c,t,!0,n)}}throw Error("Unsupported decorator location: "+i)};function Be(t){return(e,s)=>typeof s=="object"?wi(t,e,s):((i,a,o)=>{const r=a.hasOwnProperty(o);return a.constructor.createProperty(o,i),r?Object.getOwnPropertyDescriptor(a,o):void 0})(t,e,s)}/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */function M(t){return Be({...t,state:!0,attribute:!1})}/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const xi=(t,e,s)=>(s.configurable=!0,s.enumerable=!0,Reflect.decorate&&typeof e!="object"&&Object.defineProperty(t,e,s),s);/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */function fe(t,e){return(s,i,a)=>{const o=r=>r.renderRoot?.querySelector(t)??null;return xi(s,i,{get(){return o(this)}})}}/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let zs=class extends Event{constructor(e,s,i,a){super("context-request",{bubbles:!0,composed:!0}),this.context=e,this.contextTarget=s,this.callback=i,this.subscribe=a??!1}};/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 *//**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let gt=class{constructor(e,s,i,a){if(this.subscribe=!1,this.provided=!1,this.value=void 0,this.t=(o,r)=>{this.unsubscribe&&(this.unsubscribe!==r&&(this.provided=!1,this.unsubscribe()),this.subscribe||this.unsubscribe()),this.value=o,this.host.requestUpdate(),this.provided&&!this.subscribe||(this.provided=!0,this.callback&&this.callback(o,r)),this.unsubscribe=r},this.host=e,s.context!==void 0){const o=s;this.context=o.context,this.callback=o.callback,this.subscribe=o.subscribe??!1}else this.context=s,this.callback=i,this.subscribe=a??!1;this.host.addController(this)}hostConnected(){this.dispatchRequest()}hostDisconnected(){this.unsubscribe&&(this.unsubscribe(),this.unsubscribe=void 0)}dispatchRequest(){this.host.dispatchEvent(new zs(this.context,this.host,this.t,this.subscribe))}};/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */class Ci{get value(){return this.o}set value(e){this.setValue(e)}setValue(e,s=!1){const i=s||!Object.is(e,this.o);this.o=e,i&&this.updateObservers()}constructor(e){this.subscriptions=new Map,this.updateObservers=()=>{for(const[s,{disposer:i}]of this.subscriptions)s(this.o,i)},e!==void 0&&(this.value=e)}addCallback(e,s,i){if(!i)return void e(this.value);this.subscriptions.has(e)||this.subscriptions.set(e,{disposer:()=>{this.subscriptions.delete(e)},consumerHost:s});const{disposer:a}=this.subscriptions.get(e);e(this.value,a)}clearCallbacks(){this.subscriptions.clear()}}/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */class Mi extends Event{constructor(e,s){super("context-provider",{bubbles:!0,composed:!0}),this.context=e,this.contextTarget=s}}class ye extends Ci{constructor(e,s,i){super(s.context!==void 0?s.initialValue:i),this.onContextRequest=a=>{if(a.context!==this.context)return;const o=a.contextTarget??a.composedPath()[0];o!==this.host&&(a.stopPropagation(),this.addCallback(a.callback,o,a.subscribe))},this.onProviderRequest=a=>{if(a.context!==this.context||(a.contextTarget??a.composedPath()[0])===this.host)return;const o=new Set;for(const[r,{consumerHost:n}]of this.subscriptions)o.has(r)||(o.add(r),n.dispatchEvent(new zs(this.context,n,r,!0)));a.stopPropagation()},this.host=e,s.context!==void 0?this.context=s.context:this.context=s,this.attachListeners(),this.host.addController?.(this)}attachListeners(){this.host.addEventListener("context-request",this.onContextRequest),this.host.addEventListener("context-provider",this.onProviderRequest)}hostConnected(){this.host.dispatchEvent(new Mi(this.context,this.host))}}const Et="drawing-context",ki={normal:"Normal",multiply:"Multiply",screen:"Screen",overlay:"Overlay",darken:"Darken",lighten:"Lighten","soft-light":"Soft Light"},Pi={normal:"source-over",multiply:"multiply",screen:"screen",overlay:"overlay",darken:"darken",lighten:"lighten","soft-light":"soft-light"};function Yt(t){return Pi[t]}const Si={linear:t=>t,light:t=>Math.pow(t,.5),heavy:t=>Math.pow(t,2)};function $i(t){return Math.max(2,Math.round(t/2)*2)}const Ii={round:{aspect:!1,rotation:!1,bristles:!1},flat:{aspect:!0,rotation:!0,bristles:!1},chisel:{aspect:!0,rotation:!0,bristles:!1},calligraphy:{aspect:!0,rotation:!0,bristles:!1},fan:{aspect:!1,rotation:!0,bristles:!0},splatter:{aspect:!1,rotation:!0,bristles:!0}},Di={round:{shape:"round",aspect:1,angle:0,orientation:"fixed"},flat:{shape:"flat",aspect:3,angle:0,orientation:"direction"},chisel:{shape:"chisel",aspect:2.5,angle:0,orientation:"direction"},calligraphy:{shape:"calligraphy",aspect:4,angle:45,orientation:"fixed"},fan:{shape:"fan",aspect:1,angle:0,orientation:"direction",bristles:8,spread:120},splatter:{shape:"splatter",aspect:1,angle:0,orientation:"fixed",bristles:12,spread:.8}};function Ti(t){return Ii[t]}function X(t){return{...Di[t]}}const we={depletion:0,depletionLength:500,buildup:0,wetness:0},st=[{id:"round",name:"Round",category:"basic",descriptor:{size:4,opacity:1,flow:1,hardness:1,spacing:.15,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("round"),ink:{...we}}},{id:"soft-round",name:"Soft Round",category:"basic",descriptor:{size:20,opacity:1,flow:.6,hardness:.3,spacing:.12,pressureSize:!0,pressureOpacity:!0,pressureCurve:"light",tip:X("round"),ink:{...we}}},{id:"flat",name:"Flat",category:"artistic",descriptor:{size:30,opacity:1,flow:.8,hardness:.9,spacing:.1,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("flat"),ink:{depletion:.3,depletionLength:800,buildup:0,wetness:0}}},{id:"chisel",name:"Chisel",category:"artistic",descriptor:{size:24,opacity:1,flow:.9,hardness:.95,spacing:.1,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("chisel"),ink:{depletion:.2,depletionLength:600,buildup:.3,wetness:0}}},{id:"calligraphy",name:"Calligraphy",category:"artistic",descriptor:{size:20,opacity:1,flow:1,hardness:1,spacing:.08,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("calligraphy"),ink:{...we}}},{id:"fan",name:"Fan",category:"artistic",descriptor:{size:40,opacity:1,flow:.7,hardness:.8,spacing:.15,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("fan"),ink:{depletion:.5,depletionLength:600,buildup:0,wetness:0}}},{id:"splatter",name:"Splatter",category:"effects",descriptor:{size:50,opacity:.8,flow:.6,hardness:.7,spacing:.25,pressureSize:!1,pressureOpacity:!0,pressureCurve:"linear",tip:X("splatter"),ink:{depletion:.7,depletionLength:400,buildup:0,wetness:0}}},{id:"dry-brush",name:"Dry Brush",category:"artistic",descriptor:{size:25,opacity:1,flow:.5,hardness:.8,spacing:.12,pressureSize:!0,pressureOpacity:!0,pressureCurve:"heavy",tip:X("round"),ink:{depletion:.8,depletionLength:300,buildup:.4,wetness:0}}},{id:"wet-brush",name:"Wet Brush",category:"artistic",descriptor:{size:20,opacity:.8,flow:.7,hardness:.5,spacing:.1,pressureSize:!1,pressureOpacity:!1,pressureCurve:"linear",tip:X("round"),ink:{depletion:0,depletionLength:500,buildup:.2,wetness:.91}}}];function Ri(t){return st.find(e=>e.id===t)}function ie(){return{...st[0].descriptor,tip:{...st[0].descriptor.tip},ink:{...st[0].descriptor.ink}}}class N extends Error{constructor(e,s){super(e),this.cause=s,this.name="StorageError"}}class qt extends N{constructor(){super(...arguments),this.name="StorageNotFoundError"}}class As extends N{constructor(){super(...arguments),this.name="StorageQuotaError"}}class Ei extends N{constructor(){super(...arguments),this.name="StorageConflictError"}}const Li=20;class zi{constructor(e,s){this._storage=e,this._maxStamps=s?.maxStampsPerProject??Li}get storage(){return this._storage}async deleteProject(e){const s=new Set,[i,a,o,r]=await Promise.all([this._storage.state.get(e).catch(()=>null),this._storage.history.getEntries(e).catch(()=>[]),this._storage.stamps.list(e).catch(()=>[]),this._storage.projects.get(e).catch(()=>null)]);r?.thumbnailRef&&s.add(r.thumbnailRef),i?.layers.forEach(n=>s.add(n.imageBlobRef));for(const n of a)It(n.entry,s);o.forEach(n=>s.add(n.blobRef)),await this._storage.projects.delete(e),await Promise.all([this._storage.state.delete(e),this._storage.history.deleteForProject(e),this._storage.stamps.deleteForProject(e)]).catch(n=>console.error("Cascade delete partial failure:",n)),s.size>0&&this._storage.blobs.deleteMany([...s]).catch(()=>{})}async addStamp(e,s){const i=await this._storage.stamps.add(e,s),a=await this._storage.stamps.list(e);if(a.length>this._maxStamps){const o=a.filter(r=>r.id!==i.id).sort((r,n)=>r.createdAt-n.createdAt).slice(0,a.length-this._maxStamps);for(const r of o)await this._storage.stamps.delete(r.id)}return i}async collectGarbage(){if(!this._storage.blobs.gc)return 0;const e=await this._storage.projects.list(),s=new Set;for(const i of e){const[a,o,r]=await Promise.all([this._storage.state.get(i.id),this._storage.history.getEntries(i.id),this._storage.stamps.list(i.id)]);i.thumbnailRef&&s.add(i.thumbnailRef),a?.layers.forEach(n=>s.add(n.imageBlobRef)),o.forEach(n=>It(n.entry,s)),r.forEach(n=>s.add(n.blobRef)),await new Promise(n=>setTimeout(n,0))}return this._storage.blobs.gc(s)}}function It(t,e){switch(t.type){case"draw":case"patch":case"transform":e.add(t.before.blobRef),e.add(t.after.blobRef);break;case"add-layer":case"delete-layer":e.add(t.layer.imageData.blobRef);break;case"crop":case"merge":for(const s of t.beforeLayers)e.add(s.imageData.blobRef);for(const s of t.afterLayers)e.add(s.imageData.blobRef);break}}const Os="storage-backend",js="project-service";function it(){if(typeof crypto<"u"&&typeof crypto.randomUUID=="function")return crypto.randomUUID();const t=new Uint8Array(16);crypto.getRandomValues(t),t[6]=t[6]&15|64,t[8]=t[8]&63|128;const e=Array.from(t,s=>s.toString(16).padStart(2,"0")).join("");return`${e.slice(0,8)}-${e.slice(8,12)}-${e.slice(12,16)}-${e.slice(16,20)}-${e.slice(20)}`}class Ai{constructor(e){this._newId=e,this._blobs=new Map}async put(e){const s=this._newId();return this._blobs.set(s,e instanceof Blob?e:new Blob([e])),s}async get(e){const s=this._blobs.get(e);if(!s)throw new qt(`Blob ${e} not found`);return s}async delete(e){this._blobs.delete(e)}async deleteMany(e){for(const s of e)this._blobs.delete(s)}async gc(e){let s=0;for(const i of[...this._blobs.keys()])e.has(i)||(this._blobs.delete(i),s++);return s}}class Oi{constructor(e){this._newId=e,this._projects=new Map}async list(e){const s=Array.from(this._projects.values()),i=e?.orderBy??"updatedAt",a=e?.direction==="asc"?1:-1;return s.sort((o,r)=>a*(o[i]-r[i])),s}async get(e){return this._projects.get(e)??null}async create(e){const s=Date.now(),i={id:this._newId(),name:e.name,createdAt:s,updatedAt:s,thumbnailRef:e.thumbnailRef??null};return this._projects.set(i.id,i),i}async update(e,s){const i=this._projects.get(e);if(!i)throw new qt(`Project ${e}`);return s.name!==void 0&&(i.name=s.name),s.thumbnailRef!==void 0&&(i.thumbnailRef=s.thumbnailRef),i.updatedAt=Date.now(),i}async delete(e){this._projects.delete(e)}}class ji{constructor(){this._states=new Map}async get(e){return this._states.get(e)??null}async save(e){this._states.set(e.projectId,e)}async delete(e){this._states.delete(e)}}class Hi{constructor(){this._entries=new Map}async getEntries(e){return[...this._entries.get(e)??[]].sort((s,i)=>s.index-i.index)}async putEntries(e,s){const i=this._entries.get(e)??[];this._entries.set(e,[...i,...s])}async replaceAll(e,s){this._entries.set(e,[...s])}async updateEntries(e,s,i){const a=new Set(s),o=(this._entries.get(e)??[]).filter(r=>!a.has(r.index));this._entries.set(e,[...o,...i])}async deleteForProject(e){this._entries.delete(e)}}class Bi{constructor(e,s){this._blobs=e,this._newId=s,this._stamps=new Map}async list(e){return Array.from(this._stamps.values()).filter(s=>s.projectId===e).sort((s,i)=>i.createdAt-s.createdAt)}async add(e,s){const i=await this._blobs.put(s),a={id:this._newId(),projectId:e,blobRef:i,createdAt:Date.now()};return this._stamps.set(a.id,a),a}async delete(e){this._stamps.delete(e)}async deleteForProject(e){const s=[];for(const[i,a]of this._stamps)a.projectId===e&&(s.push(a.blobRef),this._stamps.delete(i));s.length>0&&this._blobs.deleteMany(s).catch(()=>{})}}class Yi{constructor(e=it){this.blobs=new Ai(e),this.projects=new Oi(e),this.state=new ji,this.history=new Hi,this.stamps=new Bi(this.blobs,e)}async init(){}async dispose(){}}function I(t){if(t instanceof DOMException)switch(t.name){case"QuotaExceededError":return new As(t.message,t);case"NotFoundError":return new qt(t.message,t);case"ConstraintError":return new Ei(t.message,t);default:return new N(t.message,t)}return t instanceof Error?new N(t.message,t):new N(String(t))}function Wi(t,e,s){const i=t.createObjectStore("blobs");if(s<1)return;function a(p){if(p?.blob){const _=it();return i.put(p.blob,_),p.blobRef=_,delete p.blob,!0}return!1}function o(p){return a(p?.imageData)}const n=e.objectStore("projects").openCursor();n.onsuccess=function(){const p=n.result;if(!p)return;const _=p.value;if(_.thumbnail){const m=it();i.put(_.thumbnail,m),_.thumbnailRef=m,delete _.thumbnail,p.update(_)}p.continue()};const h=e.objectStore("project-state").openCursor();h.onsuccess=function(){const p=h.result;if(!p)return;const _=p.value;let m=!1;for(const v of _.layers??[])if(v.imageBlob){const b=it();i.put(v.imageBlob,b),v.imageBlobRef=b,delete v.imageBlob,m=!0}m&&p.update(_),p.continue()};const d=e.objectStore("project-history").openCursor();d.onsuccess=function(){const p=d.result;if(!p)return;const _=p.value,m=_.entry;let v=!1;switch(m?.type){case"draw":case"transform":case"patch":a(m.before)&&(v=!0),a(m.after)&&(v=!0);break;case"add-layer":case"delete-layer":o(m.layer)&&(v=!0);break;case"crop":for(const b of m.beforeLayers??[])o(b)&&(v=!0);for(const b of m.afterLayers??[])o(b)&&(v=!0);break}v&&p.update(_),p.continue()};const u=e.objectStore("project-stamps").openCursor();u.onsuccess=function(){const p=u.result;if(!p)return;const _=p.value;if(_.blob){const m=it();i.put(_.blob,m),_.blobRef=m,delete _.blob,p.update(_)}p.continue()}}const Ct="blobs";class Xi{constructor(e){this._db=e}async put(e){const s=it();return await this._tx("readwrite",i=>i.put(e,s)),s}async get(e){const s=await this._tx("readonly",i=>i.get(e));if(s===void 0)throw new qt(`Blob not found: ${e}`);return s instanceof Blob?s:new Blob([s])}async delete(e){await this._tx("readwrite",s=>s.delete(e))}async deleteMany(e){e.length!==0&&await new Promise((s,i)=>{const a=this._db.transaction(Ct,"readwrite"),o=a.objectStore(Ct);for(const r of e)o.delete(r);a.oncomplete=()=>s(),a.onerror=()=>i(I(a.error))})}async gc(e){let s=0;const i=[];return await new Promise((a,o)=>{const r=this._db.transaction(Ct,"readonly"),n=r.objectStore(Ct).openKeyCursor();n.onsuccess=()=>{const c=n.result;if(c){const h=c.key;e.has(h)||i.push(h),c.continue()}},r.oncomplete=()=>a(),r.onerror=()=>o(I(r.error))}),i.length>0&&(await this.deleteMany(i),s=i.length),s}_tx(e,s){return new Promise((i,a)=>{const o=this._db.transaction(Ct,e),r=s(o.objectStore(Ct));r.onsuccess=()=>i(r.result),o.onerror=()=>a(I(o.error))})}}const K="projects";class Ui{constructor(e){this._db=e}async list(e){const s=e?.orderBy??"updatedAt",i=e?.direction??"desc";return new Promise((a,o)=>{const n=this._db.transaction(K,"readonly").objectStore(K),c=[];if(s==="updatedAt"){const l=n.index("updatedAt").openCursor(null,i==="desc"?"prev":"next");l.onsuccess=()=>{const d=l.result;d?(c.push(d.value),d.continue()):a(c)},l.onerror=()=>o(I(l.error))}else{const h=n.getAll();h.onsuccess=()=>{const l=h.result;l.sort((d,f)=>i==="desc"?f.createdAt-d.createdAt:d.createdAt-f.createdAt),a(l)},h.onerror=()=>o(I(h.error))}})}async get(e){return new Promise((s,i)=>{const o=this._db.transaction(K,"readonly").objectStore(K).get(e);o.onsuccess=()=>s(o.result??null),o.onerror=()=>i(I(o.error))})}async create(e){const s=Date.now(),i={id:it(),name:e.name,createdAt:s,updatedAt:s,thumbnailRef:e.thumbnailRef??null};return await new Promise((a,o)=>{const r=this._db.transaction(K,"readwrite");r.objectStore(K).add(i),r.oncomplete=()=>a(),r.onerror=()=>o(I(r.error))}),i}async update(e,s){return new Promise((i,a)=>{const o=this._db.transaction(K,"readwrite"),r=o.objectStore(K);let n;const c=r.get(e);c.onsuccess=()=>{const h=c.result;if(!h){a(new qt(`Project ${e} not found`));return}s.name!==void 0&&(h.name=s.name),s.thumbnailRef!==void 0&&(h.thumbnailRef=s.thumbnailRef),h.updatedAt=Date.now(),n=h,r.put(h)},c.onerror=()=>a(I(c.error)),o.oncomplete=()=>i(n),o.onerror=()=>a(I(o.error))})}async delete(e){await new Promise((s,i)=>{const a=this._db.transaction(K,"readwrite");a.objectStore(K).delete(e),a.oncomplete=()=>s(),a.onerror=()=>i(I(a.error))})}}const Mt="project-state";class Ni{constructor(e){this._db=e}async get(e){return new Promise((s,i)=>{const o=this._db.transaction(Mt,"readonly").objectStore(Mt).get(e);o.onsuccess=()=>s(o.result??null),o.onerror=()=>i(I(o.error))})}async save(e){await new Promise((s,i)=>{const a=this._db.transaction(Mt,"readwrite");a.objectStore(Mt).put(e),a.oncomplete=()=>s(),a.onerror=()=>i(I(a.error))})}async delete(e){await new Promise((s,i)=>{const a=this._db.transaction(Mt,"readwrite");a.objectStore(Mt).delete(e),a.oncomplete=()=>s(),a.onerror=()=>i(I(a.error))})}}const G="project-history";class Fi{constructor(e){this._db=e}async getEntries(e){return new Promise((s,i)=>{const o=this._db.transaction(G,"readonly").objectStore(G).index("projectId"),r=[],n=o.openCursor(IDBKeyRange.only(e));n.onsuccess=()=>{const c=n.result;c?(r.push(c.value),c.continue()):(r.sort((h,l)=>h.index-l.index),s(r))},n.onerror=()=>i(I(n.error))})}async putEntries(e,s){s.length!==0&&await new Promise((i,a)=>{const o=this._db.transaction(G,"readwrite"),r=o.objectStore(G);for(const n of s){const{id:c,...h}=n;r.add({...h,projectId:e})}o.oncomplete=()=>i(),o.onerror=()=>a(I(o.error))})}async replaceAll(e,s){await new Promise((i,a)=>{const o=this._db.transaction(G,"readwrite"),r=o.objectStore(G),c=r.index("projectId").openCursor(IDBKeyRange.only(e));c.onsuccess=()=>{const h=c.result;if(h)h.delete(),h.continue();else for(const l of s){const{id:d,...f}=l;r.add({...f,projectId:e})}},c.onerror=()=>a(I(c.error)),o.oncomplete=()=>i(),o.onerror=()=>a(I(o.error))})}async updateEntries(e,s,i){s.length===0&&i.length===0||await new Promise((a,o)=>{const r=this._db.transaction(G,"readwrite"),n=r.objectStore(G);if(s.length>0){const c=new Set(s),h=n.index("projectId").openCursor(IDBKeyRange.only(e));h.onsuccess=()=>{const l=h.result;l&&(c.has(l.value.index)&&l.delete(),l.continue())},h.onerror=()=>o(I(h.error))}for(const c of i){const{id:h,...l}=c;n.add({...l,projectId:e})}r.oncomplete=()=>a(),r.onerror=()=>o(I(r.error)),r.onabort=()=>o(I(r.error))})}async deleteForProject(e){await new Promise((s,i)=>{const a=this._db.transaction(G,"readwrite"),r=a.objectStore(G).index("projectId").openCursor(IDBKeyRange.only(e));r.onsuccess=()=>{const n=r.result;n&&(n.delete(),n.continue())},r.onerror=()=>i(I(r.error)),a.oncomplete=()=>s(),a.onerror=()=>i(I(a.error))})}}const J="project-stamps";class Vi{constructor(e,s){this._db=e,this._blobs=s}async list(e){return new Promise((s,i)=>{const o=this._db.transaction(J,"readonly").objectStore(J).index("projectId"),r=[],n=o.openCursor(IDBKeyRange.only(e));n.onsuccess=()=>{const c=n.result;c?(r.push(c.value),c.continue()):(r.sort((h,l)=>l.createdAt-h.createdAt),s(r))},n.onerror=()=>i(I(n.error))})}async add(e,s){const i=await this._blobs.put(s),a={id:it(),projectId:e,blobRef:i,createdAt:Date.now()};return await new Promise((o,r)=>{const n=this._db.transaction(J,"readwrite");n.objectStore(J).add(a),n.oncomplete=()=>o(),n.onerror=()=>r(I(n.error))}),a}async delete(e){const s=await new Promise((i,a)=>{const r=this._db.transaction(J,"readonly").objectStore(J).get(e);r.onsuccess=()=>{const n=r.result;i(n?.blobRef??null)},r.onerror=()=>a(I(r.error))});await new Promise((i,a)=>{const o=this._db.transaction(J,"readwrite");o.objectStore(J).delete(e),o.oncomplete=()=>i(),o.onerror=()=>a(I(o.error))}),s&&this._blobs.delete(s).catch(()=>{})}async deleteForProject(e){const s=[];await new Promise((i,a)=>{const o=this._db.transaction(J,"readwrite"),n=o.objectStore(J).index("projectId").openCursor(IDBKeyRange.only(e));n.onsuccess=()=>{const c=n.result;if(c){const h=c.value;s.push(h.blobRef),c.delete(),c.continue()}},n.onerror=()=>a(I(n.error)),o.oncomplete=()=>i(),o.onerror=()=>a(I(o.error))}),s.length>0&&this._blobs.deleteMany(s).catch(()=>{})}}const qi="ketchup-projects",Zi=4;class Ki{constructor(e){this._db=null,this._dbName=e?.dbName??qi,this._version=e?.version??Zi}get projects(){if(!this._projects)throw new N("Backend not initialized — call init() first");return this._projects}get state(){if(!this._state)throw new N("Backend not initialized — call init() first");return this._state}get history(){if(!this._history)throw new N("Backend not initialized — call init() first");return this._history}get stamps(){if(!this._stamps)throw new N("Backend not initialized — call init() first");return this._stamps}get blobs(){if(!this._blobs)throw new N("Backend not initialized — call init() first");return this._blobs}async init(){const e=indexedDB.deleteDatabase("ketchup-stamps");e.onerror=()=>{},e.onblocked=()=>{},this._db=await new Promise((a,o)=>{const r=indexedDB.open(this._dbName,this._version);r.onupgradeneeded=n=>{const c=r.result,h=r.transaction,l=n.oldVersion;c.objectStoreNames.contains("projects")||c.createObjectStore("projects",{keyPath:"id"}).createIndex("updatedAt","updatedAt"),c.objectStoreNames.contains("project-state")||c.createObjectStore("project-state",{keyPath:"projectId"}),c.objectStoreNames.contains("project-history")||c.createObjectStore("project-history",{keyPath:"id",autoIncrement:!0}).createIndex("projectId","projectId"),c.objectStoreNames.contains("project-stamps")||c.createObjectStore("project-stamps",{keyPath:"id"}).createIndex("projectId","projectId"),l<4&&!c.objectStoreNames.contains("blobs")&&Wi(c,h,l)},r.onsuccess=()=>a(r.result),r.onerror=()=>o(r.error)});const s=this._db,i=new Xi(s);this._blobs=i,this._projects=new Ui(s),this._state=new Ni(s),this._history=new Fi(s),this._stamps=new Vi(s,i)}async dispose(){this._db&&(this._db.close(),this._db=null)}}async function Ye(t){return new Promise((e,s)=>{t.toBlob(i=>{i?e(i):s(new Error("canvas.toBlob returned null"))},"image/png")})}async function Gi(t,e,s){const i=await createImageBitmap(t),a=document.createElement("canvas");return a.width=e,a.height=s,a.getContext("2d").drawImage(i,0,0),i.close(),a}async function Hs(t){const e=document.createElement("canvas");return e.width=t.width,e.height=t.height,e.getContext("2d").putImageData(t,0,0),Ye(e)}async function Ji(t,e,s){const i=await createImageBitmap(t),a=document.createElement("canvas");a.width=e,a.height=s;const o=a.getContext("2d");return o.drawImage(i,0,0),i.close(),o.getImageData(0,0,e,s)}function Se(t){return new Uint32Array(t.data.buffer,t.data.byteOffset,t.width*t.height)}function $e(t,e){const s=t.width,i=t.height,a=Se(t),o=Se(e);let r=s,n=-1,c=-1,h=-1;for(let l=0;l<i;l++){const d=l*s;let f=0;for(;f<s&&a[d+f]===o[d+f];)f++;if(f===s)continue;c<0&&(c=l),h=l,f<r&&(r=f);let u=s-1;for(;u>n&&a[d+u]===o[d+u];)u--;u>n&&(n=u)}return c<0?null:{x:r,y:c,w:n-r+1,h:h-c+1}}function Gt(t,e){if(e.x===0&&e.y===0&&e.w===t.width&&e.h===t.height)return t;const s=new ImageData(e.w,e.h),i=e.w*4;for(let a=0;a<e.h;a++){const o=((e.y+a)*t.width+e.x)*4;s.data.set(t.data.subarray(o,o+i),a*i)}return s}function os(t){const e=Se(t);let s=2166136261,i=2654435769;for(let a=0;a<e.length;a++){const o=e[a];s=Math.imul(s^o,16777619),s^=s>>>13,i=Math.imul(i^o,1540483477),i^=i>>>15}return`${t.width}x${t.height}:${(s>>>0).toString(16)}:${(i>>>0).toString(16)}`}async function ft(t,e){const s=await Hs(t),i=await e.put(s);return{width:t.width,height:t.height,blobRef:i}}async function _t(t,e){const s=await e.get(t.blobRef);return Ji(s,t.width,t.height)}async function kt(t,e){return{id:t.id,name:t.name,visible:t.visible,opacity:t.opacity,blendMode:t.blendMode,imageData:await ft(t.imageData,e)}}async function Pt(t,e){return{id:t.id,name:t.name,visible:t.visible,opacity:t.opacity,blendMode:t.blendMode??"normal",imageData:await _t(t.imageData,e)}}async function Qi(t,e,s){const i=await Hs(e),a=await s.put(i);return{id:t.id,name:t.name,visible:t.visible,opacity:t.opacity,blendMode:t.blendMode??"normal",imageBlobRef:a}}async function ta(t,e,s,i){const a=await i.get(t.imageBlobRef),o=await Gi(a,e,s);return{id:t.id,name:t.name,visible:t.visible,opacity:t.opacity,blendMode:t.blendMode??"normal",canvas:o}}async function ea(t,e){switch(t.type){case"draw":{const[s,i]=await Promise.all([ft(t.before,e),ft(t.after,e)]);return{type:"draw",layerId:t.layerId,before:s,after:i}}case"patch":{const[s,i]=await Promise.all([ft(t.before,e),ft(t.after,e)]);return{type:"patch",layerId:t.layerId,x:t.x,y:t.y,before:s,after:i}}case"add-layer":return{type:"add-layer",layer:await kt(t.layer,e),index:t.index};case"delete-layer":return{type:"delete-layer",layer:await kt(t.layer,e),index:t.index};case"crop":{const[s,i]=await Promise.all([Promise.all(t.beforeLayers.map(a=>kt(a,e))),Promise.all(t.afterLayers.map(a=>kt(a,e)))]);return{type:"crop",beforeLayers:s,afterLayers:i,beforeWidth:t.beforeWidth,beforeHeight:t.beforeHeight,afterWidth:t.afterWidth,afterHeight:t.afterHeight}}case"merge":{const[s,i]=await Promise.all([Promise.all(t.beforeLayers.map(a=>kt(a,e))),Promise.all(t.afterLayers.map(a=>kt(a,e)))]);return{type:"merge",beforeLayers:s,afterLayers:i,previousActiveLayerId:t.previousActiveLayerId,afterActiveLayerId:t.afterActiveLayerId}}case"reorder":case"visibility":case"opacity":case"rename":case"blend-mode":return t;case"transform":{const[s,i]=await Promise.all([ft(t.before,e),ft(t.after,e)]);return{type:"transform",layerId:t.layerId,before:s,after:i}}}}async function sa(t,e){switch(t.type){case"draw":{const[s,i]=await Promise.all([_t(t.before,e),_t(t.after,e)]);return{type:"draw",layerId:t.layerId,before:s,after:i}}case"patch":{const[s,i]=await Promise.all([_t(t.before,e),_t(t.after,e)]);return{type:"patch",layerId:t.layerId,x:t.x,y:t.y,before:s,after:i}}case"add-layer":return{type:"add-layer",layer:await Pt(t.layer,e),index:t.index};case"delete-layer":return{type:"delete-layer",layer:await Pt(t.layer,e),index:t.index};case"crop":{const[s,i]=await Promise.all([Promise.all(t.beforeLayers.map(a=>Pt(a,e))),Promise.all(t.afterLayers.map(a=>Pt(a,e)))]);return{type:"crop",beforeLayers:s,afterLayers:i,beforeWidth:t.beforeWidth,beforeHeight:t.beforeHeight,afterWidth:t.afterWidth,afterHeight:t.afterHeight}}case"merge":{const[s,i]=await Promise.all([Promise.all(t.beforeLayers.map(a=>Pt(a,e))),Promise.all(t.afterLayers.map(a=>Pt(a,e)))]);return{type:"merge",beforeLayers:s,afterLayers:i,previousActiveLayerId:t.previousActiveLayerId,afterActiveLayerId:t.afterActiveLayerId}}case"reorder":case"visibility":case"opacity":case"rename":return t;case"blend-mode":return{type:"blend-mode",layerId:t.layerId,before:t.before,after:t.after};case"transform":{const[s,i]=await Promise.all([_t(t.before,e),_t(t.after,e)]);return{type:"transform",layerId:t.layerId,before:s,after:i}}}}const jt={select:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square" stroke-linejoin="miter">
      <rect x="4" y="4" width="16" height="16" stroke-dasharray="4 4" />
    </svg>`,move:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="5 9 2 12 5 15"/>
      <polyline points="9 5 12 2 15 5"/>
      <polyline points="15 19 12 22 9 19"/>
      <polyline points="19 9 22 12 19 15"/>
      <line x1="2" y1="12" x2="22" y2="12"/>
      <line x1="12" y1="2" x2="12" y2="22"/>
    </svg>`,pencil:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M17 3a2.83 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>
    </svg>`,eraser:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m7 21-4.3-4.3c-1-1-1-2.5 0-3.4l9.6-9.6c1-1 2.5-1 3.4 0l5.6 5.6c1 1 1 2.5 0 3.4L13 21"/>
      <path d="M22 21H7"/>
      <path d="m5 11 9 9"/>
    </svg>`,line:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <line x1="5" y1="19" x2="19" y2="5"/>
    </svg>`,rectangle:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2"/>
    </svg>`,circle:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="12" cy="12" r="10"/>
    </svg>`,triangle:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 3L22 21H2z"/>
    </svg>`,diamond:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2L22 12 12 22 2 12z"/>
    </svg>`,pentagon:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2L22 9.3 18.2 21H5.8L2 9.3z"/>
    </svg>`,hexagon:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M7 2H17L22 12 17 22H7L2 12z"/>
    </svg>`,star:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2.5l2.9 5.9 6.5.9-4.7 4.6 1.1 6.5-5.8-3-5.8 3 1.1-6.5-4.7-4.6 6.5-.9z"/>
    </svg>`,heart:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8l1.1 1.1L12 21.2l7.8-7.7 1.1-1.1a5.5 5.5 0 0 0-.1-7.8z"/>
    </svg>`,arrow:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 9h10V4l8 8-8 8v-5H3z"/>
    </svg>`,fill:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m19 11-8-8-8.6 8.6a2 2 0 0 0 0 2.8l5.2 5.2c.8.8 2 .8 2.8 0L19 11Z"/>
      <path d="m5 2 5 5"/>
      <path d="M2 13h15"/>
      <path d="M22 20a2 2 0 1 1-4 0c0-1.6 1.7-2.4 2-4 .3 1.6 2 2.4 2 4Z"/>
    </svg>`,stamp:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M2 22h20"/>
      <path d="M6 12v-4c0-2.2 1.8-4 4-4h4c2.2 0 4 1.8 4 4v4"/>
      <path d="M6 12h12a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-2a2 2 0 0 1 2-2Z"/>
      <path d="M9 13v-1"/>
      <path d="M15 13v-1"/>
    </svg>`,eyedropper:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m2 22 1-1h3l9-9"/>
      <path d="M3 21v-3l9-9"/>
      <path d="m15 6 3.4-3.4a2.1 2.1 0 1 1 3 3L18 9"/>
      <path d="m15 6 3 3"/>
    </svg>`,text:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="4 7 4 4 20 4 20 7"/>
      <line x1="12" y1="4" x2="12" y2="21"/>
      <line x1="8" y1="21" x2="16" y2="21"/>
    </svg>`,hand:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M18 11V6a2 2 0 0 0-4 0v1"/>
      <path d="M14 10V4a2 2 0 0 0-4 0v6"/>
      <path d="M10 10.5V6a2 2 0 0 0-4 0v8"/>
      <path d="M18 8a2 2 0 0 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/>
    </svg>`,crop:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M6 2v4H2"/>
      <path d="M6 6h12v12"/>
      <path d="M18 22v-4h4"/>
      <path d="M2 6h4v12h12"/>
    </svg>`},rs=$`
  <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M12 2.5 17 10H7z"/>
    <circle cx="6.5" cy="17" r="4"/>
    <rect x="13" y="13" width="8" height="8" rx="1"/>
  </svg>`,W={undo:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="1 4 1 10 7 10"/>
      <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/>
    </svg>`,redo:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="23 4 23 10 17 10"/>
      <path d="M20.49 15a9 9 0 1 1-2.13-9.36L23 10"/>
    </svg>`,save:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
      <polyline points="7 10 12 15 17 10"/>
      <line x1="12" y1="15" x2="12" y2="3"/>
    </svg>`,clear:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 6h18"/>
      <path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/>
      <path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>
    </svg>`,exitChildMode:$`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/>
      <polyline points="10 17 15 12 10 7"/>
      <line x1="15" y1="12" x2="3" y2="12"/>
    </svg>`},We={select:"V",move:"M",hand:"H",pencil:"B",eraser:"E",line:"L",rectangle:"U",circle:"C",triangle:"T",diamond:"D",pentagon:"P",hexagon:"K",star:"A",heart:"J",arrow:"Q",fill:"G",stamp:"S",text:"X",crop:"R",eyedropper:"I"},ia=new Map(Object.entries(We).map(([t,e])=>[e.toLowerCase(),t]));function aa(t){return ia.get(t.toLowerCase())}const nt={select:"Select",move:"Move",pencil:"Pencil",eraser:"Eraser",line:"Line",rectangle:"Rectangle",circle:"Circle",triangle:"Triangle",diamond:"Diamond",pentagon:"Pentagon",hexagon:"Hexagon",star:"Star",heart:"Heart",arrow:"Arrow",fill:"Fill",stamp:"Stamp",text:"Text",hand:"Hand (Pan)",crop:"Crop",eyedropper:"Eyedropper"},Bs=["pencil","eraser","rectangle","circle","triangle","fill"],oa=new Set(Bs),Ie=120,De=8,Te=1600;function ns(t,e=Ie){return Number.isFinite(t)?Math.max(De,Math.min(Te,t)):e}const _e=["rectangle","circle","line","triangle","diamond","pentagon","hexagon","star","heart","arrow"];function ht(t){return _e.includes(t)}function et(t,e){e&&t.fill(),t.stroke()}function ra(t,e,s,i,a,o){const r=s+a/2,n=i+o/2,c=a/2,h=o/2;for(let l=0;l<e;l++){const d=-Math.PI/2+l*Math.PI*2/e,f=r+Math.cos(d)*c,u=n+Math.sin(d)*h;l===0?t.moveTo(f,u):t.lineTo(f,u)}t.closePath()}function cs(t,e,s,i,a,o,r,n){const c=Math.min(s.x,i.x),h=Math.min(s.y,i.y),l=Math.abs(i.x-s.x),d=Math.abs(i.y-s.y);switch(t.save(),t.strokeStyle=a,t.lineWidth=n,t.lineCap="round",t.lineJoin="round",r&&(t.fillStyle=o),e){case"line":t.beginPath(),t.moveTo(s.x,s.y),t.lineTo(i.x,i.y),t.stroke();break;case"rectangle":{t.beginPath(),t.rect(c,h,l,d),et(t,r);break}case"circle":{const f=c+l/2,u=h+d/2;t.beginPath(),t.ellipse(f,u,l/2,d/2,0,0,Math.PI*2),et(t,r);break}case"triangle":{t.beginPath(),t.moveTo(c+l/2,h),t.lineTo(c+l,h+d),t.lineTo(c,h+d),t.closePath(),et(t,r);break}case"diamond":{t.beginPath(),t.moveTo(c+l/2,h),t.lineTo(c+l,h+d/2),t.lineTo(c+l/2,h+d),t.lineTo(c,h+d/2),t.closePath(),et(t,r);break}case"pentagon":{t.beginPath(),ra(t,5,c,h,l,d),et(t,r);break}case"hexagon":{t.beginPath(),t.moveTo(c+l*.25,h),t.lineTo(c+l*.75,h),t.lineTo(c+l,h+d/2),t.lineTo(c+l*.75,h+d),t.lineTo(c+l*.25,h+d),t.lineTo(c,h+d/2),t.closePath(),et(t,r);break}case"star":{const f=c+l/2,u=h+d/2,p=l/2,_=d/2,m=.42;t.beginPath();for(let v=0;v<10;v++){const b=-Math.PI/2+v*Math.PI/5,y=v%2===0?1:m,x=f+Math.cos(b)*p*y,C=u+Math.sin(b)*_*y;v===0?t.moveTo(x,C):t.lineTo(x,C)}t.closePath(),et(t,r);break}case"heart":{t.beginPath(),t.moveTo(c+l/2,h+d),t.bezierCurveTo(c+l*.12,h+d*.72,c,h+d*.42,c,h+d*.27),t.bezierCurveTo(c,h+d*.04,c+l*.38,h,c+l/2,h+d*.2),t.bezierCurveTo(c+l*.62,h,c+l,h+d*.04,c+l,h+d*.27),t.bezierCurveTo(c+l,h+d*.42,c+l*.88,h+d*.72,c+l/2,h+d),t.closePath(),et(t,r);break}case"arrow":{t.beginPath(),t.moveTo(c,h+d*.3),t.lineTo(c+l*.58,h+d*.3),t.lineTo(c+l*.58,h),t.lineTo(c+l,h+d/2),t.lineTo(c+l*.58,h+d),t.lineTo(c+l*.58,h+d*.7),t.lineTo(c,h+d*.7),t.closePath(),et(t,r);break}}t.restore()}function dt(t,e){if(typeof OffscreenCanvas<"u")return new OffscreenCanvas(t,e);const s=document.createElement("canvas");return s.width=t,s.height=e,s}function tt(t,e){return t.getContext("2d",e)}function lt(t,e,s,i,a,o){a!==void 0&&o!==void 0?t.drawImage(e,s,i,a,o):t.drawImage(e,s,i)}function Xe(t,e,s,i){t.globalCompositeOperation="source-in",t.fillStyle=e,t.fillRect(0,0,s,i),t.globalCompositeOperation="source-over"}const ls=X("fan"),hs=X("splatter");function na(t,e,s){const i=Math.max(1,t),a=dt(i,i),o=tt(a),r=i/2;if(e>=1)o.fillStyle="#fff",o.beginPath(),o.arc(r,r,r,0,Math.PI*2),o.fill();else{const n=o.createRadialGradient(r,r,r*e,r,r,r);n.addColorStop(0,"rgba(255,255,255,1)"),n.addColorStop(1,"rgba(255,255,255,0)"),o.fillStyle=n,o.beginPath(),o.arc(r,r,r,0,Math.PI*2),o.fill()}return a}function ca(t,e,s){const i=Math.max(1,t),a=Math.max(1,Math.ceil(t/Math.max(1,s.aspect))),o=dt(i,a),r=tt(o);if(e>=1)r.fillStyle="#fff",r.fillRect(0,0,i,a);else{r.fillStyle="#fff",r.fillRect(0,0,i,a);const n=Math.max(1,(1-e)*Math.min(i,a)*.5);r.globalCompositeOperation="destination-in";const c=r.createLinearGradient(0,0,i,0);c.addColorStop(0,"rgba(255,255,255,0)"),c.addColorStop(Math.min(.5,n/i),"rgba(255,255,255,1)"),c.addColorStop(Math.max(.5,1-n/i),"rgba(255,255,255,1)"),c.addColorStop(1,"rgba(255,255,255,0)"),r.fillStyle=c,r.fillRect(0,0,i,a);const h=r.createLinearGradient(0,0,0,a);h.addColorStop(0,"rgba(255,255,255,0)"),h.addColorStop(Math.min(.5,n/a),"rgba(255,255,255,1)"),h.addColorStop(Math.max(.5,1-n/a),"rgba(255,255,255,1)"),h.addColorStop(1,"rgba(255,255,255,0)"),r.fillStyle=h,r.fillRect(0,0,i,a),r.globalCompositeOperation="source-over"}return o}function la(t,e,s){const i=Math.max(1,t),a=Math.max(1,Math.ceil(t/Math.max(1,s.aspect))),o=dt(i,a),r=tt(o),n=a/3;if(r.beginPath(),r.moveTo(n,0),r.lineTo(i,0),r.lineTo(i-n,a),r.lineTo(0,a),r.closePath(),e>=1)r.fillStyle="#fff",r.fill();else{r.fillStyle="#fff",r.fill();const c=i/2,h=a/2,l=Math.sqrt(c*c+h*h);r.globalCompositeOperation="destination-in";const d=r.createRadialGradient(c,h,l*e,c,h,l);d.addColorStop(0,"rgba(255,255,255,1)"),d.addColorStop(1,"rgba(255,255,255,0)"),r.fillStyle=d,r.fillRect(0,0,i,a),r.globalCompositeOperation="source-over"}return o}function ha(t,e,s){const i=Math.max(1,t/2),a=Math.max(1,t/Math.max(1,s.aspect)/2),o=Math.max(1,t),r=Math.max(1,Math.ceil(a*2)),n=dt(o,r),c=tt(n),h=o/2,l=r/2;if(e>=1)c.fillStyle="#fff",c.beginPath(),c.ellipse(h,l,i,a,0,0,Math.PI*2),c.fill();else{c.save(),c.translate(h,l),c.scale(1,a/i);const d=c.createRadialGradient(0,0,i*e,0,0,i);d.addColorStop(0,"rgba(255,255,255,1)"),d.addColorStop(1,"rgba(255,255,255,0)"),c.fillStyle=d,c.beginPath(),c.arc(0,0,i,0,Math.PI*2),c.fill(),c.restore()}return n}function Ys(t){let e=t|0;return()=>{e=e+1831565813|0;let s=Math.imul(e^e>>>15,1|e);return s=s+Math.imul(s^s>>>7,61|s)^s,((s^s>>>14)>>>0)/4294967296}}function Ws(t,e,s,i=0){const a=s.bristles??ls.bristles,r=(s.spread??ls.spread)*Math.PI/180,n=t/2,c=Math.max(1,t/8),h=dt(t,t),l=tt(h),d=t/2,f=t/2,u=-Math.PI/2-r/2,p=Ys(42+i*7);for(let _=0;_<a;_++){const m=a>1?_/(a-1):.5,v=u+r*m,b=n*.08*(p()-.5),y=.05*(p()-.5),x=d+Math.cos(v+y)*(n-c+b),C=f+Math.sin(v+y)*(n-c+b);if(e>=1)l.fillStyle="#fff",l.beginPath(),l.arc(x,C,c,0,Math.PI*2),l.fill();else{const k=l.createRadialGradient(x,C,c*e,x,C,c);k.addColorStop(0,"rgba(255,255,255,1)"),k.addColorStop(1,"rgba(255,255,255,0)"),l.fillStyle=k,l.beginPath(),l.arc(x,C,c,0,Math.PI*2),l.fill()}}return h}function Xs(t,e,s,i=0){const a=s.bristles??hs.bristles,o=s.spread??hs.spread,r=t/2*o,n=Math.max(1,t/10),c=dt(t,t),h=tt(c),l=t/2,d=t/2,f=Ys(137+i*13);for(let u=0;u<a;u++){const p=f()*Math.PI*2,_=f()*r,m=l+Math.cos(p)*_,v=d+Math.sin(p)*_,b=n*(.5+f()*.5);if(e>=1)h.fillStyle="#fff",h.beginPath(),h.arc(m,v,b,0,Math.PI*2),h.fill();else{const y=h.createRadialGradient(m,v,b*e,m,v,b);y.addColorStop(0,"rgba(255,255,255,1)"),y.addColorStop(1,"rgba(255,255,255,0)"),h.fillStyle=y,h.beginPath(),h.arc(m,v,b,0,Math.PI*2),h.fill()}}return c}const ds={round:na,flat:ca,chisel:la,calligraphy:ha,fan:Ws,splatter:Xs},da={fan:4,splatter:6},pa=128;class ua{constructor(){this._entries=new Map,this._accessCounter=0}_buildKey(e,s,i,a){let o=`${i.shape}-${e}-${s.toFixed(2)}-${i.aspect.toFixed(1)}`;return i.bristles!=null&&(o+=`-b${i.bristles}`),i.spread!=null&&(o+=`-s${i.spread.toFixed(2)}`),a!=null&&(o+=`-v${a}`),o}get(e,s,i){const a=this._buildKey(e,s,i),o=this._entries.get(a);if(o)return o.lastUsed=++this._accessCounter,o.canvas;const r=ds[i.shape],n=r(e,s,i);return this._entries.set(a,{canvas:n,key:a,lastUsed:++this._accessCounter}),this._evictIfNeeded(),n}getVariant(e,s,i,a){const o=this._buildKey(e,s,i,a),r=this._entries.get(o);if(r)return r.lastUsed=++this._accessCounter,r.canvas;let n;return i.shape==="fan"?n=Ws(e,s,i,a):i.shape==="splatter"?n=Xs(e,s,i,a):n=ds[i.shape](e,s,i),this._entries.set(o,{canvas:n,key:o,lastUsed:++this._accessCounter}),this._evictIfNeeded(),n}_evictIfNeeded(){for(;this._entries.size>pa;){let e=null,s=1/0;for(const[i,a]of this._entries)a.lastUsed<s&&(s=a.lastUsed,e=i);if(e)this._entries.delete(e);else break}}clear(){this._entries.clear(),this._accessCounter=0}}class fa{constructor(){this._canvas=null,this._width=0,this._height=0}acquire(e,s){return(!this._canvas||e>this._width||s>this._height)&&(this._width=Math.max(this._width,e),this._height=Math.max(this._height,s),this._canvas=dt(this._width,this._height)),tt(this._canvas).clearRect(0,0,this._width,this._height),this._canvas}commit(e,s,i,a,o,r,n=!1){if(this._canvas)if(a)e.save(),e.globalAlpha=i,e.globalCompositeOperation="destination-out",lt(e,this._canvas,0,0),e.restore();else if(n)e.save(),e.globalAlpha=i,e.globalCompositeOperation="source-over",lt(e,this._canvas,0,0),e.restore();else{const c=tt(this._canvas);Xe(c,s,o,r),e.save(),e.globalAlpha=i,e.globalCompositeOperation="source-over",lt(e,this._canvas,0,0),e.restore()}}get current(){return this._canvas}}const xe=1;class _a{constructor(){this._window=[],this._count=0,this._remainder=0}reset(){this._window=[],this._count=0,this._remainder=0}addPoint(e,s,i,a,o=0){const r={x:e,y:s,pressure:i,timestamp:o};this._count++,this._window.push(r),this._window.length>4&&this._window.shift();const n=this._count;if(n===1)return this._remainder=a,[{x:e,y:s,pressure:i,speedPxPerMs:xe}];if(n===2)return this._walkLinear(this._window[0],this._window[1],a);if(n===3)return[];const[c,h,l,d]=this._window;return this._walkCatmullRom(c,h,l,d,a)}flush(e){const s=this._count,i=this._window;if(s<2)return[];if(s===2)return[];if(s===3)return this._walkLinear(i[i.length-2],i[i.length-1],e);const a=i[1],o=i[2],r=i[3],n={x:r.x+(r.x-o.x),y:r.y+(r.y-o.y),pressure:r.pressure,timestamp:r.timestamp+Math.max(0,r.timestamp-o.timestamp)};return this._walkCatmullRom(a,o,r,n,e)}_walkLinear(e,s,i){const a=s.x-e.x,o=s.y-e.y,r=Math.sqrt(a*a+o*o);if(r<.001)return[];const n=[],c=s.timestamp-e.timestamp,h=c>0?r/c:xe;let l=this._remainder;for(;l<=r;){const d=l/r;n.push({x:e.x+a*d,y:e.y+o*d,pressure:e.pressure+(s.pressure-e.pressure)*d,speedPxPerMs:h}),l+=i}return this._remainder=l-r,n}_walkCatmullRom(e,s,i,a,o){let n=0,c=s.x,h=s.y;const l=[0];for(let _=1;_<=20;_++){const m=_/20,v=Jt(e.x,s.x,i.x,a.x,m),b=Jt(e.y,s.y,i.y,a.y,m),y=Math.sqrt((v-c)**2+(b-h)**2);n+=y,l.push(n),c=v,h=b}if(n<.001)return[];const d=[],f=i.timestamp-s.timestamp,u=f>0?n/f:xe;let p=this._remainder;for(;p<=n;){let _=0;for(let x=1;x<l.length;x++)if(l[x]>=p){_=x-1;break}const m=l[_],v=l[_+1],b=v>m?(p-m)/(v-m):0,y=(_+b)/20;d.push({x:Jt(e.x,s.x,i.x,a.x,y),y:Jt(e.y,s.y,i.y,a.y,y),pressure:s.pressure+(i.pressure-s.pressure)*y,speedPxPerMs:u}),p+=o}return this._remainder=p-n,d}}function Jt(t,e,s,i,a){const o=a*a,r=o*a;return .5*(2*e+(-t+s)*a+(2*t-5*e+4*s-i)*o+(-t+3*e-3*s+i)*r)}function Ce(t){return t<=.04045?t/12.92:Math.pow((t+.055)/1.055,2.4)}function Me(t){return t<=.0031308?t*12.92:1.055*Math.pow(t,1/2.4)-.055}function ps(t,e,s){const i=Ce(t),a=Ce(e),o=Ce(s),r=Math.cbrt(.4122214708*i+.5363325363*a+.0514459929*o),n=Math.cbrt(.2119034982*i+.6806995451*a+.1073969566*o),c=Math.cbrt(.0883024619*i+.2817188376*a+.6299787005*o);return{L:.2104542553*r+.793617785*n-.0040720468*c,a:1.9779984951*r-2.428592205*n+.4505937099*c,b:.0259040371*r+.7827717662*n-.808675766*c}}function ma(t,e,s){const i=t+.3963377774*e+.2158037573*s,a=t-.1055613458*e-.0638541728*s,o=t-.0894841775*e-1.291485548*s,r=i*i*i,n=a*a*a,c=o*o*o;return{r:Math.max(0,Math.min(1,Me(4.0767416621*r-3.3077115913*n+.2309699292*c))),g:Math.max(0,Math.min(1,Me(-1.2684380046*r+2.6097574011*n-.3413193965*c))),b:Math.max(0,Math.min(1,Me(-.0041960863*r-.7034186147*n+1.707614701*c)))}}function us(t){const e=parseInt(t.slice(1,7),16);return{r:(e>>16&255)/255,g:(e>>8&255)/255,b:(e&255)/255}}function ga(t){const e=Math.round(t.r*255),s=Math.round(t.g*255),i=Math.round(t.b*255);return`#${(1<<24|e<<16|s<<8|i).toString(16).slice(1)}`}function va(t,e,s){if(s<=0)return t;if(s>=1)return e;const i=us(t),a=us(e),o=ps(i.r,i.g,i.b),r=ps(a.r,a.g,a.b),n=ma(o.L+(r.L-o.L)*s,o.a+(r.a-o.a)*s,o.b+(r.b-o.b)*s);return ga(n)}function ba(t,e){return{distanceTraveled:0,remainingPaint:1,originalColor:t,currentColor:t,stampCount:0,layerSnapshot:e,prevRotation:0}}function ya(t,e){if(t.depletion<=0)return 1;const s=Math.max(0,1-e.distanceTraveled/Math.max(1,t.depletionLength)*t.depletion);return e.remainingPaint=s,s}const wa=1;function xa(t,e,s){if(t.buildup<=0)return e;const a=1-Math.min(1,Math.max(0,s/wa));return Math.min(1,e*(1+t.buildup*a*3))}const Ca=48;function Ma(t,e,s,i=6){const a=t.width,o=t.height,r=t.data,n=Math.round(e),c=Math.round(s),h=Math.max(1,Math.round(i)),l=h*h,d=Math.max(1,Math.ceil((2*h+1)/Ca));let f=0,u=0,p=0,_=0,m=0;const v=Math.max(0,n-h),b=Math.min(a-1,n+h),y=Math.max(0,c-h),x=Math.min(o-1,c+h);for(let E=y;E<=x;E+=d){const H=E-c;for(let B=v;B<=b;B+=d){const Lt=B-n;if(Lt*Lt+H*H>l)continue;const q=(E*a+B)*4,Z=r[q+3];Z<2||(f+=r[q]*Z,u+=r[q+1]*Z,p+=r[q+2]*Z,_+=Z,m++)}}if(m===0||_===0)return{color:"#000000",alpha:0};const C=Math.round(f/_),k=Math.round(u/_),T=Math.round(p/_),S=Math.round(_/m);return{color:`#${(1<<24|C<<16|k<<8|T).toString(16).slice(1)}`,alpha:S}}function ka(t,e,s,i,a=6){if(t.wetness<=0||!e.layerSnapshot)return;const o=Ma(e.layerSnapshot,s,i,Math.max(4,a));if(o.alpha<10){e.currentColor=e.originalColor;return}const r=o.alpha/255;e.currentColor=va(e.originalColor,o.color,t.wetness*r)}const Pa=.25;function Sa(t,e,s,i){if(i.orientation==="fixed"||!e)return i.angle*Math.PI/180;const a=t.x-e.x,o=t.y-e.y;return a*a+o*o<Pa?s:Math.atan2(o,a)+i.angle*Math.PI/180}class Us{constructor(){this._tipCache=new ua,this._bufferPool=new fa,this._smoother=new _a,this._descriptor=null,this._color="",this._eraser=!1,this._colorMode=!1,this._docWidth=0,this._docHeight=0,this._lastMappedPressure=.5,this._inkState=null,this._prevStamp=null,this._variantCounter=0,this._snapshotCaptured=!1,this._tintCanvas=null,this._tintW=0,this._tintH=0,this._dirtyMinX=1/0,this._dirtyMinY=1/0,this._dirtyMaxX=-1/0,this._dirtyMaxY=-1/0}begin(e,s,i,a,o){this._descriptor={...e,tip:{...e.tip},ink:{...e.ink}},this._color=i?"":s.length===9?s.slice(0,7):s,this._eraser=i,this._colorMode=!i&&e.ink.wetness>0,this._docWidth=a,this._docHeight=o,this._bufferPool.acquire(a,o),this._smoother.reset(),this._prevStamp=null,this._variantCounter=0,this._snapshotCaptured=!1,this._dirtyMinX=1/0,this._dirtyMinY=1/0,this._dirtyMaxX=-1/0,this._dirtyMaxY=-1/0,this._inkState=ba(this._color,null)}stroke(e,s,i,a,o=0){if(!this._descriptor||!this._inkState)return;const r=this._descriptor;let n=i;if(!Number.isFinite(i)||i<=0){if(r.pressureSize||r.pressureOpacity){this._smoother.reset(),this._prevStamp=null;return}n=1}this._colorMode&&!this._snapshotCaptured&&a&&(this._inkState.layerSnapshot=a.getImageData(0,0,this._docWidth,this._docHeight),this._snapshotCaptured=!0);const c=Si[r.pressureCurve],h=c(n);this._lastMappedPressure=h;const l=r.pressureSize?Math.max(1,r.size*h):r.size,d=Math.max(1,r.spacing*l),f=this._smoother.addPoint(e,s,h,d,o);this._stampPoints(f)}_stampPoints(e){if(!this._descriptor||!this._inkState)return;const s=this._descriptor,i=s.ink,a=this._inkState,o=this._bufferPool.current;if(!o)return;const r=tt(o),n=da[s.tip.shape]??0;for(const c of e){let h=0;if(this._prevStamp){const C=c.x-this._prevStamp.x,k=c.y-this._prevStamp.y;h=Math.sqrt(C*C+k*k),a.distanceTraveled+=h}a.stampCount++;const l=ya(i,a);if(l<=0){this._prevStamp=c;continue}const d=s.pressureOpacity?s.flow*c.pressure:s.flow,f=xa(i,d,c.speedPxPerMs),u=s.pressureSize?Math.max(1,s.size*c.pressure):s.size;ka(i,a,c.x,c.y,u/2);const p=$i(u);let _;if(n>0){const C=this._variantCounter%n;_=this._tipCache.getVariant(p,s.hardness,s.tip,C),this._variantCounter++}else _=this._tipCache.get(p,s.hardness,s.tip);const m=Sa(c,this._prevStamp,a.prevRotation,s.tip);a.prevRotation=m;const v=Math.min(1,f*l),b=_.width,y=_.height,x=Math.sqrt(b*b+y*y)/2+1;if(c.x-x<this._dirtyMinX&&(this._dirtyMinX=c.x-x),c.y-x<this._dirtyMinY&&(this._dirtyMinY=c.y-x),c.x+x>this._dirtyMaxX&&(this._dirtyMaxX=c.x+x),c.y+x>this._dirtyMaxY&&(this._dirtyMaxY=c.y+x),this._colorMode){(b>this._tintW||y>this._tintH)&&(this._tintW=Math.max(this._tintW,b),this._tintH=Math.max(this._tintH,y),this._tintCanvas=dt(this._tintW,this._tintH));const C=tt(this._tintCanvas);C.globalCompositeOperation="source-over",C.clearRect(0,0,this._tintW,this._tintH),lt(C,_,0,0),Xe(C,a.currentColor,b,y),r.globalAlpha=v,r.globalCompositeOperation="source-over",m!==0?(r.save(),r.translate(Math.round(c.x),Math.round(c.y)),r.rotate(m),lt(r,this._tintCanvas,-b/2,-y/2,b,y),r.restore()):lt(r,this._tintCanvas,Math.round(c.x-b/2),Math.round(c.y-y/2),b,y)}else r.globalAlpha=v,r.globalCompositeOperation="source-over",m!==0?(r.save(),r.translate(Math.round(c.x),Math.round(c.y)),r.rotate(m),lt(r,_,-b/2,-y/2,b,y),r.restore()):lt(r,_,Math.round(c.x-b/2),Math.round(c.y-y/2),b,y);this._prevStamp=c}r.globalAlpha=1}commit(e){if(!this._descriptor)return!1;const s=this._descriptor.pressureSize?Math.max(1,this._descriptor.size*this._lastMappedPressure):this._descriptor.size,i=Math.max(1,this._descriptor.spacing*s),a=this._smoother.flush(i);return a.length>0&&this._stampPoints(a),this._bufferPool.commit(e,this._color,this._descriptor.opacity,this._eraser,this._docWidth,this._docHeight,this._colorMode),this._descriptor=null,this._inkState=null,!0}cancel(){this._descriptor=null,this._inkState=null,this._smoother.reset()}getDirtyBounds(){if(this._dirtyMaxX<this._dirtyMinX)return null;const e=Math.max(0,Math.floor(this._dirtyMinX)),s=Math.max(0,Math.floor(this._dirtyMinY)),i=Math.min(this._docWidth,Math.ceil(this._dirtyMaxX))-e,a=Math.min(this._docHeight,Math.ceil(this._dirtyMaxY))-s;return i<=0||a<=0?null:{x:e,y:s,w:i,h:a}}getStrokePreview(){return!this._descriptor||!this._bufferPool.current?null:{canvas:this._bufferPool.current,eraser:this._eraser,opacity:this._descriptor.opacity,color:this._colorMode?null:this._color,bounds:this.getDirtyBounds()}}}const $a=96,Ia=100,Q=new Map,Ht=new Map,Dt=new Map;function fs(t,e){const s=Q.get(t);for(s&&s!==e&&URL.revokeObjectURL(s),Q.delete(t),Q.set(t,e);Q.size>Ia;){const i=Q.entries().next().value;if(!i)break;Q.delete(i[0]),URL.revokeObjectURL(i[1])}return e}async function Da(t){const e=await createImageBitmap(t);try{const s=Math.min(1,$a/Math.max(e.width,e.height)),i=document.createElement("canvas");return i.width=Math.max(1,Math.round(e.width*s)),i.height=Math.max(1,Math.round(e.height*s)),i.getContext("2d").drawImage(e,0,0,i.width,i.height),URL.createObjectURL(await Ye(i))}finally{e.close()}}async function Ta(t,e){const s=Q.get(e.id);if(s)return fs(e.id,s);const i=Ht.get(e.id);if(i)return i;const a=Dt.get(e.id)??0,o=(async()=>{const r=await t.blobs.get(e.blobRef),n=await Da(r);if((Dt.get(e.id)??0)!==a)throw URL.revokeObjectURL(n),new Error("Stamp thumbnail was removed while loading.");return fs(e.id,n)})();Ht.set(e.id,o);try{return await o}finally{Ht.get(e.id)===o&&(Ht.delete(e.id),Q.has(e.id)||Dt.delete(e.id))}}function Ra(t){const e=Q.get(t);e&&URL.revokeObjectURL(e),Q.delete(t),Ht.has(t)?Dt.set(t,(Dt.get(t)??0)+1):Dt.delete(t)}var Ea=Object.defineProperty,La=Object.getOwnPropertyDescriptor,Y=(t,e,s,i)=>{for(var a=i>1?void 0:i?La(e,s):e,o=t.length-1,r;o>=0;o--)(r=t[o])&&(a=(i?r(e,s,a):r(a))||a);return i&&a&&Ea(e,s,a),a};const _s={rename:$`<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M17 3a2.83 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/></svg>`,delete:$`<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>`},za=$`<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/></svg>`,Aa=[{label:"800 × 600",width:800,height:600},{label:"1024 × 768",width:1024,height:768},{label:"1280 × 720 (HD)",width:1280,height:720},{label:"1920 × 1080 (Full HD)",width:1920,height:1080},{label:"2560 × 1440 (QHD)",width:2560,height:1440},{label:"A4 Portrait (794 × 1123)",width:794,height:1123},{label:"A4 Landscape (1123 × 794)",width:1123,height:794},{label:"Square 1024",width:1024,height:1024}],Oa=["#000000","#ffffff","#ff0000","#ff6600","#ffcc00","#33cc33","#0099ff","#6633ff","#cc33cc","#996633","#ff9999","#ffcc99","#ffff99","#99ff99","#99ccff","#cc99ff","#cccccc","#666666"];let j=class extends F{constructor(){super(...arguments),this._aspectLock=!1,this._recentStamps=[],this._stampBusy=!1,this._stampMessage="",this._stampMessageError=!1,this._projectDropdownOpen=!1,this._advancedOpen=!1,this._brushDropdownOpen=!1,this._openPanel=null,this._refocusAfterClose=!1,this._previewCache=new Map,this._customPreviewKey="",this._customPreviewUrl="",this._previewCanvas=null,this._previewSampleCanvas=null,this._previewStrokeCanvas=null,this._previewEngine=new Us,this._renamingProjectId=null,this._newProjectName="Untitled",this._newProjectWidth="800",this._newProjectHeight="600",this._thumbUrls=new Map,this._lastProjectId=null,this._stampLoadVersion=0,this._ctx=new gt(this,{context:Et,subscribe:!0}),this._storageCtx=new gt(this,{context:Os,subscribe:!0}),this._serviceCtx=new gt(this,{context:js,subscribe:!0}),this._onDocumentClick=t=>{if(this._projectDropdownOpen){const e=t.composedPath(),s=this.shadowRoot?.querySelector(".project-dropdown-wrap");s&&!e.includes(s)&&this._closeDropdown()}},this._onPanelResize=()=>this._clampOpenPanel(),this._onPanelOutsidePointer=t=>{const e=this._openPanelWrap();e&&!t.composedPath().includes(e)&&this._closePanel()},this._onPanelEscape=t=>{if(t.key!=="Escape"||this._openPanel===null)return;t.stopPropagation();const e=this._openPanelWrap(),s=!!e&&t.composedPath().includes(e);this._closePanel(),s&&e.querySelector(".panel-trigger")?.focus()},this._onPanelFocusOut=t=>{const e=t.relatedTarget;if(!e)return;t.currentTarget.contains(e)||this._closePanel()},this._onBrushDropdownOutsideClick=t=>{const e=t.composedPath(),s=this.shadowRoot?.querySelector(".brush-dropdown-wrap");s&&!e.includes(s)&&this._closeBrushDropdown()}}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback()}willUpdate(){if(this.toggleAttribute("mobile",this._ctx.value?.isMobile??!1),this._openPanel&&!this._panelAvailable(this._openPanel)){const e=this._openPanelWrap();this._refocusAfterClose=!!e&&e.contains(this.shadowRoot.activeElement),this._closePanel()}const t=this._ctx.value?.currentProject?.id??null;t&&t!==this._lastProjectId&&(this._lastProjectId=t,this._recentStamps=[],this._thumbUrls.clear(),this._stampMessage="",this._stampMessageError=!1,this._loadStamps(t))}disconnectedCallback(){super.disconnectedCallback(),this._closeDropdown(),document.removeEventListener("click",this._onBrushDropdownOutsideClick),this._closePanel(),this._stampLoadVersion++,this._thumbUrls.clear()}async _loadStamps(t){const e=this._storageCtx.value;if(!e)return;const s=++this._stampLoadVersion;try{const i=await e.stamps.list(t);if(this._lastProjectId!==t||s!==this._stampLoadVersion)return;this._recentStamps=i;const a=new Set(i.map(o=>o.id));for(const o of this._thumbUrls.keys())a.has(o)||this._thumbUrls.delete(o);for(const o of i){if(this._lastProjectId!==t||s!==this._stampLoadVersion)return;this._thumbUrls.has(o.id)||(this._thumbUrls.set(o.id,await Ta(e,o)),this._lastProjectId===t&&s===this._stampLoadVersion&&(this._recentStamps=[...i]))}this._lastProjectId===t&&s===this._stampLoadVersion&&(this._recentStamps=i)}catch(i){this._lastProjectId===t&&s===this._stampLoadVersion&&(this._stampMessage=i instanceof Error?i.message:"Could not load recent stamps.",this._stampMessageError=!0)}}_onStrokeColor(t){this.ctx.setStrokeColor(t.target.value)}_onFillColor(t){this.ctx.setFillColor(t.target.value)}_onBrushSize(t){this.ctx.setBrushSize(Number(t.target.value))}_onStampSize(t){this.ctx.setStampSize(Number(t.target.value))}_onUseFill(t){this.ctx.setUseFill(t.target.checked)}_uploadStamp(){if(!this._ctx.value?.currentProject?.id)return;const e=document.createElement("input");e.type="file",e.accept="image/*",e.multiple=!0,e.onchange=async()=>{const s=e.files;if(!s||s.length===0)return;const i=this._ctx.value?.currentProject?.id;if(!i)return;const a=this._serviceCtx.value;if(!a)return;const o=10*1024*1024,r=4096;let n=null,c=0,h=0;this._stampBusy=!0,this._stampMessage=`Importing ${s.length===1?s[0].name:`${s.length} images`}…`,this._stampMessageError=!1;try{for(const l of s){if(this._ctx.value?.currentProject?.id!==i)break;if(l.size>o){h++;continue}try{const d=await createImageBitmap(l);if(d.width>r||d.height>r){h++,d.close();continue}d.close()}catch{h++;continue}n=await a.addStamp(i,l),c++}await this._loadStamps(i),n&&this._lastProjectId===i&&(this._stampBusy=!1,await this._selectStamp(n,!1)),this._stampMessage=c>0?`${c} ${c===1?"stamp":"stamps"} added${h?`; ${h} skipped (10 MB / 4096 px limit).`:"."}`:"No stamps were added. Use a valid image up to 10 MB and 4096 px per side.",this._stampMessageError=c===0}catch(l){this._stampMessage=l instanceof Error?l.message:"Could not add the stamp.",this._stampMessageError=!0}finally{this._stampBusy=!1}},e.click()}async _selectStamp(t,e=!0){const s=this._storageCtx.value,i=this._ctx.value?.currentProject?.id??null;if(!(!s||this._stampBusy||!i||t.projectId!==i||this._lastProjectId!==i)){this._stampBusy=!0,e&&(this._stampMessage="Loading stamp…",this._stampMessageError=!1);try{const a=await s.blobs.get(t.blobRef),o=URL.createObjectURL(a);let r;try{r=await new Promise((n,c)=>{const h=new Image;h.onload=()=>n(h),h.onerror=()=>c(new Error("The stamp image could not be decoded.")),h.src=o})}finally{URL.revokeObjectURL(o)}if(this._lastProjectId!==i||this._ctx.value?.currentProject?.id!==i)return;this.ctx.setStampImage(r,t.id),e&&(this._stampMessage="Stamp selected. Click the canvas to place it.")}catch(a){this._stampMessage=a instanceof Error?a.message:"Could not load the stamp.",this._stampMessageError=!0}finally{this._stampBusy=!1}}}async _deleteStamp(t,e){e.stopPropagation();const s=this._ctx.value?.currentProject?.id;if(!s||t.projectId!==s||this._lastProjectId!==s)return;const i=this._storageCtx.value;if(i)try{if(await i.stamps.delete(t.id),Ra(t.id),this._ctx.value?.currentProject?.id!==s||this._lastProjectId!==s)return;this.ctx.state.activeStampId===t.id&&this.ctx.setStampImage(null,null),await this._loadStamps(s),this._stampMessage="Stamp removed.",this._stampMessageError=!1}catch(a){this._stampMessage=a instanceof Error?a.message:"Could not remove the stamp.",this._stampMessageError=!0}}_closeDropdown(){this._projectDropdownOpen&&(this._projectDropdownOpen=!1,document.removeEventListener("click",this._onDocumentClick))}_toggleProjectDropdown(){this._projectDropdownOpen?this._closeDropdown():(this._projectDropdownOpen=!0,document.addEventListener("click",this._onDocumentClick))}_onSelectProject(t){this._closeDropdown(),this.ctx.switchProject(t)}_onNewProject(){this._closeDropdown(),this._newProjectName="Untitled",this._newProjectWidth="800",this._newProjectHeight="600",this.shadowRoot?.querySelector(".new-project-dialog")?.showModal(),this.updateComplete.then(()=>{const e=this.shadowRoot?.querySelector(".new-project-name-input");e&&(e.focus(),e.select())})}_cancelNewProject(){this.shadowRoot?.querySelector(".new-project-dialog")?.close()}_confirmNewProject(){const t=this._newProjectName.trim()||"Untitled",e=parseInt(this._newProjectWidth),s=parseInt(this._newProjectHeight);if(!e||!s||e<=0||s<=0||e>8192||s>8192)return;this.shadowRoot?.querySelector(".new-project-dialog")?.close(),this.ctx.createProject(t,e,s)}_selectNewProjectPreset(t){this._newProjectWidth=String(t.width),this._newProjectHeight=String(t.height)}_onNewProjectKeydown(t){t.stopPropagation(),t.key==="Enter"&&this._confirmNewProject()}_onDeleteProject(t,e){t.stopPropagation(),confirm("Delete this project? This cannot be undone.")&&(this._closeDropdown(),this.ctx.deleteProject(e))}_startRename(t,e){t.stopPropagation(),this._renamingProjectId=e,this.updateComplete.then(()=>{const s=this.shadowRoot?.querySelector(".project-rename-input");s&&(s.focus(),s.select())})}_onRenameKeydown(t,e){t.stopPropagation(),t.key==="Enter"?this._commitRename(t,e):t.key==="Escape"&&(this._renamingProjectId=null)}_commitRename(t,e){if(this._renamingProjectId!==e)return;const i=t.target.value.trim();i&&this.ctx.renameProject(e,i),this._renamingProjectId=null}_showsShapeOptions(){const t=this.ctx.state.activeTool;return ht(t)&&t!=="line"}_generatePreview(t,e=!1){const s=`${e?"eraser":"paint"}:${t.id}`,i=this._previewCache.get(s);if(i)return i;const a=this._generateDescriptorPreview(t.descriptor,e);return this._previewCache.set(s,a),a}_generateCustomPreview(t,e=!1){const s=`${e?"eraser":"paint"}:${JSON.stringify(t)}`;return s===this._customPreviewKey?this._customPreviewUrl:(this._customPreviewKey=s,this._customPreviewUrl=this._generateDescriptorPreview(t,e),this._customPreviewUrl)}_generateDescriptorPreview(t,e=!1){const a=this._previewCanvas??=document.createElement("canvas");a.width!==160&&(a.width=160),a.height!==48&&(a.height=48);const o=a.getContext("2d");o.globalAlpha=1,o.globalCompositeOperation="source-over",o.clearRect(0,0,160,48),o.fillStyle="#2a2a2a",o.fillRect(0,0,160,48);const r=Math.max(2,Math.min(18,2+Math.log2(Math.max(1,t.size))*2.5)),n={...t,size:r,tip:{...t.tip},ink:{...t.ink}},c=8,h=r/2+2,l=Math.max(2,(48/2-h)*.8),d=48/2,f=160-c*2,u=this._previewSampleCanvas??=document.createElement("canvas");u.width!==160&&(u.width=160),u.height!==48&&(u.height=48);const p=u.getContext("2d");p.globalAlpha=1,p.globalCompositeOperation="source-over",p.clearRect(0,0,160,48),p.fillStyle="#ef6c57",p.fillRect(0,0,160/2,48),p.fillStyle="#5b8cf7",p.fillRect(160/2,0,160/2,48);const _=this._previewStrokeCanvas??=document.createElement("canvas");_.width!==160&&(_.width=160),_.height!==48&&(_.height=48);const m=_.getContext("2d");m.globalAlpha=1,m.globalCompositeOperation="source-over",m.clearRect(0,0,160,48),e&&(m.fillStyle="#cccccc",m.fillRect(0,0,160,48)),this._previewEngine.begin(n,"#cccccc",e,160,48);let v=0;for(let b=c;b<=160-c;b+=1){const y=(b-c)/f,x=d+l*Math.sin(y*Math.PI*2),C=.4+.6*Math.sin(y*Math.PI);v+=y>.35&&y<.65?4:1;const k=!e&&n.ink.wetness>0?p:void 0;this._previewEngine.stroke(b,x,C,k,v)}return this._previewEngine.commit(m),o.drawImage(_,0,0),a.toDataURL()}updated(){this._refocusAfterClose&&(this._refocusAfterClose=!1,this.shadowRoot?.querySelector(".panel-trigger, .project-name-btn")?.focus()),this._clampOpenPanel()}_clampOpenPanel(){const t=this._openPanelWrap()?.querySelector(".settings-panel");if(!t)return;t.style.left="";const e=8,s=t.getBoundingClientRect(),i=s.right-(window.innerWidth-e);i>0&&(t.style.left=`${-Math.min(i,Math.max(0,s.left-e))}px`)}_panelAvailable(t){const e=this._ctx.value;if(!e||e.isMobile)return!1;const s=e.state.activeTool;return t==="brush"?s==="pencil"||s==="eraser":s!=="select"&&s!=="eraser"&&s!=="stamp"}_togglePanel(t){if(this._openPanel===t){this._closePanel();return}this._openPanel=t,document.addEventListener("pointerdown",this._onPanelOutsidePointer,!0),document.addEventListener("keydown",this._onPanelEscape,!0),window.addEventListener("resize",this._onPanelResize)}_closePanel(){this._openPanel!==null&&(this._openPanel=null,document.removeEventListener("pointerdown",this._onPanelOutsidePointer,!0),document.removeEventListener("keydown",this._onPanelEscape,!0),window.removeEventListener("resize",this._onPanelResize))}_openPanelWrap(){return this._openPanel===null?null:this.shadowRoot?.querySelector(`.panel-wrap[data-panel="${this._openPanel}"]`)??null}_renderColorControls(t){const e=g`
      <input
        type="color"
        .value=${t}
        @input=${this._onStrokeColor}
        title="Stroke color"
        aria-label="Stroke color"
      />
    `,s=g`
      <div class="color-grid">
        ${Oa.map(i=>g`
            <button
              class="color-swatch ${t===i?"active":""}"
              style="background:${i}"
              title=${i}
              aria-label=${`Use ${i}`}
              @click=${()=>this.ctx.setStrokeColor(i)}
            ></button>
          `)}
      </div>
    `;return this.ctx.isMobile?g`${e}${s}`:g`
      ${s}
      <label class="custom-color">${e} Custom color…</label>
    `}_toggleBrushDropdown(){this._brushDropdownOpen=!this._brushDropdownOpen,this._brushDropdownOpen?requestAnimationFrame(()=>{document.addEventListener("click",this._onBrushDropdownOutsideClick)}):document.removeEventListener("click",this._onBrushDropdownOutsideClick)}_closeBrushDropdown(){this._brushDropdownOpen=!1,document.removeEventListener("click",this._onBrushDropdownOutsideClick)}_selectPreset(t){this.ctx.selectPreset(t),this._closeBrushDropdown()}_renderTransformSettings(){const t=this._ctx.value?.getTransformValues();if(!t)return g`<div class="section"><label>No transform active</label></div>`;const{x:e,y:s,width:i,height:a,rotation:o,skewX:r,skewY:n,flipH:c,flipV:h}=t,l=(f,u)=>this._ctx.value?.setTransformValue(f,u),d=(f,u)=>p=>{const _=p.target.value,m=parseFloat(_);if(!isNaN(m))if(f==="width"&&this._aspectLock&&i!==0){const v=a/i;l("width",m),l("height",Math.round(m*v*10)/10)}else if(f==="height"&&this._aspectLock&&a!==0){const v=i/a;l("height",m),l("width",Math.round(m*v*10)/10)}else l(f,m)};return g`
      <div class="transform-section">
        <label>Position</label>
        <div class="transform-row">
          <span class="transform-suffix">X</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(e*10)/10)}
            aria-label="X position" @change=${d("x")} />
          <span class="transform-suffix">Y</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(s*10)/10)}
            aria-label="Y position" @change=${d("y")} />
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Size</label>
        <div class="transform-row">
          <span class="transform-suffix">W</span>
          <input class="transform-input" type="number" step="0.1" min="1"
            .value=${String(Math.round(i*10)/10)}
            aria-label="Width" @change=${d("width")} />
          <button
            class="aspect-lock-btn ${this._aspectLock?"active":""}"
            title="Lock aspect ratio"
            @click=${()=>{this._aspectLock=!this._aspectLock}}
          >
            <svg viewBox="0 0 10 14" width="10" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              ${this._aspectLock?g`<rect x="1" y="5" width="8" height="8" rx="1"/><path d="M3 5V3.5a2 2 0 1 1 4 0V5"/>`:g`<rect x="1" y="5" width="8" height="8" rx="1"/><path d="M3 5V3.5a2 2 0 1 1 4 0V4" stroke-dasharray="2 1"/>`}
            </svg>
          </button>
          <span class="transform-suffix">H</span>
          <input class="transform-input" type="number" step="0.1" min="1"
            .value=${String(Math.round(a*10)/10)}
            aria-label="Height" @change=${d("height")} />
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Rotation</label>
        <div class="transform-row">
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(o*10)/10)}
            aria-label="Rotation in degrees" @change=${d("rotation","°")} />
          <span class="transform-suffix">°</span>
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Skew</label>
        <div class="transform-row">
          <span class="transform-suffix">X</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(r*10)/10)}
            aria-label="Horizontal skew in degrees" @change=${d("skewX","°")} />
          <span class="transform-suffix">°</span>
          <span class="transform-suffix" style="margin-left:0.25rem;">Y</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(n*10)/10)}
            aria-label="Vertical skew in degrees" @change=${d("skewY","°")} />
          <span class="transform-suffix">°</span>
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Flip</label>
        <div class="transform-row">
          <button
            class="flip-btn ${c?"active":""}"
            title="Flip Horizontal"
            @click=${()=>l("flipH",!0)}
          >
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M8 2v12M2 5l4 3-4 3M14 5l-4 3 4 3"/>
            </svg>
          </button>
          <button
            class="flip-btn ${h?"active":""}"
            title="Flip Vertical"
            @click=${()=>l("flipV",!0)}
          >
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M2 8h12M5 2l3 4 3-4M5 14l3-4 3 4"/>
            </svg>
          </button>
        </div>
      </div>
    `}_renderBrushDetails(){const{brush:t,activeTool:e}=this.ctx.state,s=Ti(t.tip.shape),i=X(t.tip.shape),a=!this.ctx.isMobile,o=n=>a?g`<div class="panel-heading">${n}</div>`:P,r=g`
      <button
        class="advanced-toggle"
        aria-expanded=${this._advancedOpen?"true":"false"}
        @click=${()=>{this._advancedOpen=!this._advancedOpen}}
      >Advanced ${this._advancedOpen?g`&#9650;`:g`&#9660;`}</button>
    `;return g`
        ${o("Stroke")}
        <div class="section">
          <label>Flow</label>
          <input type="range" aria-label="Brush flow" min="1" max="100" .value=${String(Math.round(t.flow*100))}
            @input=${n=>this.ctx.setBrush({flow:Number(n.target.value)/100})} />
          <span class="size-value">${Math.round(t.flow*100)}%</span>
        </div>
        <div class="section">
          <label>Hardness</label>
          <input type="range" aria-label="Brush hardness" min="0" max="100" .value=${String(Math.round(t.hardness*100))}
            @input=${n=>this.ctx.setBrush({hardness:Number(n.target.value)/100})} />
          <span class="size-value">${Math.round(t.hardness*100)}%</span>
        </div>
        <div class="section">
          <label>Spacing</label>
          <input type="range" min="5" max="100" aria-label="Spacing" .value=${String(Math.round(t.spacing*100))}
            @input=${n=>this.ctx.setBrush({spacing:Number(n.target.value)/100})} />
          <span class="size-value">${Math.round(t.spacing*100)}%</span>
        </div>
        ${o("Stylus")}
        <div class="section">
          <label class="checkbox-label" title="Affects pressure-sensitive pen or stylus input; mouse input uses full pressure.">
            <input type="checkbox" .checked=${t.pressureSize}
              @change=${n=>this.ctx.setBrush({pressureSize:n.target.checked})} />
            Stylus Size
          </label>
        </div>
        <div class="section">
          <label class="checkbox-label" title="Affects pressure-sensitive pen or stylus input; mouse input uses full pressure.">
            <input type="checkbox" .checked=${t.pressureOpacity}
              @change=${n=>this.ctx.setBrush({pressureOpacity:n.target.checked})} />
            Stylus Opacity
          </label>
        </div>
        ${t.pressureSize||t.pressureOpacity?g`
        <div class="section">
          <label title="Maps pressure-sensitive pen or stylus input; mouse input uses full pressure.">Stylus Curve</label>
          <select class="font-select" aria-label="Stylus curve" .value=${t.pressureCurve}
            @change=${n=>this.ctx.setBrush({pressureCurve:n.target.value})}>
            <option value="linear">Linear</option>
            <option value="light">Light</option>
            <option value="heavy">Heavy</option>
          </select>
        </div>
        `:P}
        <div class="section">${r}</div>
        ${this._advancedOpen?g`
          <div class="section advanced-group">
            <div class="section">
              <label>Tip</label>
              <div class="pill-row">
                ${["round","flat","chisel","calligraphy","fan","splatter"].map(n=>g`
                  <button
                    class="pill-btn ${t.tip.shape===n?"active":""}"
                    @click=${()=>{t.tip.shape!==n&&this.ctx.setBrushTip(X(n))}}
                  >${n.charAt(0).toUpperCase()+n.slice(1)}</button>
                `)}
              </div>
            </div>
            ${s.aspect?g`
            <div class="section">
              <label>Aspect</label>
              <input type="range" min="1" max="6" step="0.5" .value=${String(t.tip.aspect)}
                @input=${n=>this.ctx.setBrushTip({aspect:Number(n.target.value)})} />
              <span class="size-value">${t.tip.aspect}</span>
            </div>
            `:P}
            ${s.rotation?g`
            <div class="section">
              <label>${t.tip.orientation==="direction"?"Offset":"Angle"}</label>
              <input type="range" min="0" max="360" .value=${String(t.tip.angle)}
                @input=${n=>this.ctx.setBrushTip({angle:Number(n.target.value)})} />
              <span class="size-value">${t.tip.angle}&deg;</span>
            </div>
            <div class="section">
              <label>Orient</label>
              <select class="font-select" aria-label="Tip orientation" .value=${t.tip.orientation}
                @change=${n=>this.ctx.setBrushTip({orientation:n.target.value})}>
                <option value="fixed">Fixed</option>
                <option value="direction">Direction</option>
              </select>
            </div>
            `:P}
            ${s.bristles?g`
              <div class="section">
                <label>Bristles</label>
                <input type="range" min="1" max="20" .value=${String(t.tip.bristles??i.bristles)}
                  @input=${n=>this.ctx.setBrushTip({bristles:Number(n.target.value)})} />
                <span class="size-value">${t.tip.bristles??i.bristles}</span>
              </div>
              <div class="section">
                <label>Spread</label>
                <input type="range" min="0" max="200" .value=${String(Math.round((t.tip.spread??i.spread)*(t.tip.shape==="fan"?1:100)))}
                  @input=${n=>{const c=Number(n.target.value);this.ctx.setBrushTip({spread:t.tip.shape==="fan"?c:c/100})}} />
                <span class="size-value">${t.tip.shape==="fan"?t.tip.spread??i.spread:Math.round((t.tip.spread??i.spread)*100)+"%"}</span>
              </div>
            `:P}
            <div class="section">
              <label>Depletion</label>
              <input type="range" min="0" max="100" .value=${String(Math.round(t.ink.depletion*100))}
                @input=${n=>this.ctx.setBrushInk({depletion:Number(n.target.value)/100})} />
              <span class="size-value">${Math.round(t.ink.depletion*100)}%</span>
            </div>
            ${t.ink.depletion>0?g`
              <div class="section">
                <label>Depl. Len</label>
                <input type="range" min="100" max="2000" .value=${String(t.ink.depletionLength)}
                  @input=${n=>this.ctx.setBrushInk({depletionLength:Number(n.target.value)})} />
                <span class="size-value">${t.ink.depletionLength}px</span>
              </div>
            `:P}
            ${t.flow<1||t.pressureOpacity?g`
            <div class="section">
              <label>Buildup</label>
              <input type="range" min="0" max="100" .value=${String(Math.round(t.ink.buildup*100))}
                @input=${n=>this.ctx.setBrushInk({buildup:Number(n.target.value)/100})} />
              <span class="size-value">${Math.round(t.ink.buildup*100)}%</span>
            </div>
            `:P}
            ${e!=="eraser"?g`
            <div class="section">
              <label>Wetness</label>
              <input type="range" min="0" max="100" .value=${String(Math.round(t.ink.wetness*100))}
                @input=${n=>this.ctx.setBrushInk({wetness:Number(n.target.value)/100})} />
              <span class="size-value">${Math.round(t.ink.wetness*100)}%</span>
            </div>
            `:P}
          </div>
        `:P}
    `}render(){if(!this._ctx.value)return g``;const t=this.ctx.state,{strokeColor:e,fillColor:s,useFill:i,activeTool:a,stampImage:o,stampSize:r,brush:n}=t;if(a==="select")return this._ctx.value.transformActive?this._renderTransformSettings():g`
        <div class="section" style="padding:16px;color:#888;font-size:12px;text-align:center;line-height:1.5;">
          Draw a selection to transform, or press
          <kbd style="background:#333;padding:1px 5px;border-radius:3px;font-size:11px;">${navigator.platform?.startsWith("Mac")?"⌘":"Ctrl"}+T</kbd>
          to transform the entire layer.
        </div>
      `;const c=n.size,h=this.ctx.isMobile;return g`
      ${h?"":g`
        ${this.ctx.embedded?P:g`
        <div class="section project-section">
          <div class="project-dropdown-wrap">
            <button class="project-name-btn" @click=${this._toggleProjectDropdown}>
              ${this.ctx.currentProject?.name??"Untitled"}
              <span class="dropdown-arrow">&#9662;</span>
            </button>
            ${this._projectDropdownOpen?g`
              <div class="project-dropdown">
                ${this.ctx.projectList.map(l=>g`
                  <div class="project-item ${l.id===this.ctx.currentProject?.id?"active":""}">
                    ${this._renamingProjectId===l.id?g`
                      <input
                        class="project-rename-input"
                        aria-label="Project name"
                        .value=${l.name}
                        @keydown=${d=>this._onRenameKeydown(d,l.id)}
                        @blur=${d=>this._commitRename(d,l.id)}
                      />
                    `:g`
                      <span class="project-item-name" @click=${()=>this._onSelectProject(l.id)}>
                        ${l.name}
                      </span>
                      <button
                        class="project-item-action"
                        title="Rename project"
                        aria-label=${`Rename ${l.name}`}
                        @click=${d=>this._startRename(d,l.id)}
                      >${_s.rename}</button>
                      <button
                        class="project-item-action delete"
                        title="Delete project"
                        aria-label=${`Delete ${l.name}`}
                        @click=${d=>this._onDeleteProject(d,l.id)}
                      >${_s.delete}</button>
                    `}
                  </div>
                `)}
                <div class="project-dropdown-divider"></div>
                <button class="project-new-btn" @click=${this._onNewProject}>+ New Project</button>
              </div>
            `:""}
          </div>
        </div>
        `}
        <div class="section document-size" title="Canvas size">
          ${this.ctx.state.documentWidth} \u00d7 ${this.ctx.state.documentHeight}
        </div>
        <div class="separator"></div>
      `}

      ${ht(a)?g`
        <div class="section">
          <label>Shape</label>
          <div class="shape-picker" role="group" aria-label="Shape">
            ${_e.map(l=>g`
              <button
                class="shape-option ${a===l?"active":""}"
                data-shape=${l}
                title=${`${nt[l]} (${We[l]})`}
                aria-label=${`Select ${nt[l]} shape`}
                aria-pressed=${a===l?"true":"false"}
                @click=${()=>this.ctx.setTool(l)}
              >${jt[l]}</button>
            `)}
          </div>
        </div>
        <div class="separator"></div>
      `:P}

      ${a!=="eraser"&&a!=="stamp"?g`
      ${h?g`
        <div class="section">
          <label>Color</label>
          ${this._renderColorControls(e)}
        </div>
      `:g`
        <div class="section panel-wrap" data-panel="color" @focusout=${this._onPanelFocusOut}>
          <button
            class="panel-trigger color-trigger ${this._openPanel==="color"?"open":""}"
            title="Color"
            aria-label="Color"
            aria-haspopup="dialog"
            aria-controls="color-panel"
            aria-expanded=${this._openPanel==="color"?"true":"false"}
            @click=${()=>this._togglePanel("color")}
          >
            <span class="color-chip" style="background:${e}"></span>
            <span class="chevron">&#9660;</span>
          </button>
          ${this._openPanel==="color"?g`
            <div id="color-panel" class="settings-panel color-panel" role="dialog" aria-label="Color">
              <div class="panel-heading">Color</div>
              ${this._renderColorControls(e)}
            </div>
          `:P}
        </div>
      `}

      <div class="separator"></div>
      `:P}

      <div class="section">
        <label>${a==="stamp"?"Stamp size":"Size"}</label>
        <input
          type="range"
          min=${a==="stamp"?De:1}
          max=${a==="stamp"?Te:150}
          aria-label=${a==="stamp"?"Stamp size":"Brush size"}
          .value=${String(a==="stamp"?r:c)}
          @input=${a==="stamp"?this._onStampSize:this._onBrushSize}
        />
        ${a==="stamp"?g`
              <input
                class="stamp-size-input"
                type="number"
                min=${De}
                max=${Te}
                step="1"
                aria-label="Stamp size in pixels"
                title="Stamp size in pixels"
                .value=${String(r)}
                @change=${this._onStampSize}
              />
            `:g`<span class="size-value">${c}</span>`}
      </div>

      ${a==="pencil"||a==="eraser"?g`
        <div class="separator"></div>
        <div class="section">
          <div class="brush-dropdown-wrap">
            <button class="brush-dropdown-btn" @click=${()=>this._toggleBrushDropdown()}>
              <img src=${t.isPresetModified?this._generateCustomPreview(n,a==="eraser"):this._generatePreview(st.find(l=>l.id===t.activePreset)??st[0],a==="eraser")} alt="" />
              <span>${(st.find(l=>l.id===t.activePreset)??st[0]).name}${t.isPresetModified?" *":""}</span>
              <span class="chevron">&#9660;</span>
            </button>
            ${this._brushDropdownOpen?g`
              <div class="brush-dropdown-panel">
                ${st.map(l=>g`
                  <button
                    class="brush-dropdown-item ${t.activePreset===l.id&&!t.isPresetModified?"active":""}"
                    @click=${()=>this._selectPreset(l.id)}
                  >
                    <img src=${this._generatePreview(l,a==="eraser")} alt="" />
                    <span>${l.name}</span>
                  </button>
                `)}
              </div>
            `:P}
          </div>
        </div>
        <div class="separator"></div>
        <div class="section">
          <label>Opacity</label>
          <input type="range" aria-label="Brush opacity" min="0" max="100" .value=${String(Math.round(n.opacity*100))}
            @input=${l=>this.ctx.setBrush({opacity:Number(l.target.value)/100})} />
          <span class="size-value">${Math.round(n.opacity*100)}%</span>
        </div>
        ${h?this._renderBrushDetails():g`
          <div class="section panel-wrap" data-panel="brush" @focusout=${this._onPanelFocusOut}>
            <button
              class="panel-trigger ${this._openPanel==="brush"?"open":""}"
              title="Brush settings"
              aria-label="Brush settings"
              aria-haspopup="dialog"
              aria-controls="brush-panel"
              aria-expanded=${this._openPanel==="brush"?"true":"false"}
              @click=${()=>this._togglePanel("brush")}
            >
              ${za}
              <span class="brush-trigger-label">Brush settings</span>
              <span class="chevron">&#9660;</span>
            </button>
            ${this._openPanel==="brush"?g`
              <div id="brush-panel" class="settings-panel brush-panel" role="dialog" aria-label="Brush settings">
                ${this._renderBrushDetails()}
              </div>
            `:P}
          </div>
        `}
      `:P}

      ${a==="eyedropper"?g`
        <div class="section">
          <label class="checkbox-label">
            <input type="checkbox" .checked=${t.eyedropperSampleAll}
              @change=${l=>this.ctx.setEyedropperSampleAll(l.target.checked)} />
            Sample all layers
          </label>
        </div>
      `:P}

      ${this._showsShapeOptions()?g`
            <div class="separator"></div>
            <div class="section">
              <label class="checkbox-label">
                <input type="checkbox" .checked=${i} @change=${this._onUseFill} />
                Fill
              </label>
              ${i?g`
                    <input
                      type="color"
                      .value=${s}
                      @input=${this._onFillColor}
                      title="Fill color"
                    />
                  `:""}
            </div>
          `:""}

      ${a==="crop"?g`
            <div class="separator"></div>
            <div class="section">
              <label>Ratio</label>
              <select
                .value=${this.ctx.state.cropAspectRatio}
                @change=${l=>this.ctx.setCropAspectRatio(l.target.value)}
                style="background:#444;color:#ddd;border:1px solid #555;border-radius:0.25rem;padding:0.25rem 0.375rem;font-size:0.8125rem;cursor:pointer;"
              >
                <option value="free">Free</option>
                <option value="1:1">1:1</option>
                <option value="4:3">4:3</option>
                <option value="3:2">3:2</option>
                <option value="16:9">16:9</option>
                <option value="3:4">3:4</option>
                <option value="2:3">2:3</option>
                <option value="9:16">9:16</option>
              </select>
            </div>
          `:""}

      ${a==="text"?g`
            <div class="separator"></div>
            <div class="section">
              <label>Font</label>
              <select
                class="font-select"
                aria-label="Font"
                .value=${this.ctx.state.fontFamily}
                @change=${l=>this.ctx.setFontFamily(l.target.value)}
              >
                <option value="sans-serif">Sans-serif</option>
                <option value="serif">Serif</option>
                <option value="monospace">Monospace</option>
                <option value="Arial">Arial</option>
                <option value="Georgia">Georgia</option>
                <option value="Courier New">Courier New</option>
                <option value="Verdana">Verdana</option>
                <option value="Times New Roman">Times New Roman</option>
              </select>
            </div>
            <div class="section">
              <label>Size</label>
              <input
                class="font-size-input"
                type="number"
                min="8"
                max="200"
                .value=${String(this.ctx.state.fontSize)}
                @change=${l=>this.ctx.setFontSize(Number(l.target.value))}
              />
            </div>
            <div class="section">
              <button
                class="font-toggle ${this.ctx.state.fontBold?"active":""}"
                title="Bold"
                @click=${()=>this.ctx.setFontBold(!this.ctx.state.fontBold)}
              ><strong>B</strong></button>
              <button
                class="font-toggle ${this.ctx.state.fontItalic?"active":""}"
                title="Italic"
                @click=${()=>this.ctx.setFontItalic(!this.ctx.state.fontItalic)}
              ><em>I</em></button>
            </div>
          `:""}

      ${a==="stamp"?g`
            <div class="stamp-line">
              <button
                class="stamp-btn"
                @click=${this._uploadStamp}
                title="Upload one or more stamp images"
                aria-label="Upload one or more stamp images"
                ?disabled=${this._stampBusy}
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                  <polyline points="17 8 12 3 7 8"/>
                  <line x1="12" y1="3" x2="12" y2="15"/>
                </svg>
              </button>
              ${this._recentStamps.length>0?g`
                    <div class="stamp-row">
                      ${this._recentStamps.map((l,d)=>g`
                          <div class="stamp-thumb-wrap">
                            <button
                              class="stamp-thumb ${t.activeStampId===l.id?"active":""}"
                              aria-label=${`Select recent stamp ${d+1}`}
                              aria-pressed=${t.activeStampId===l.id?"true":"false"}
                              title=${`Select recent stamp ${d+1}`}
                              @click=${()=>this._selectStamp(l)}
                              ?disabled=${this._stampBusy}
                            >
                              <img
                                src=${this._thumbUrls.get(l.id)??""}
                                alt=""
                                loading="lazy"
                                decoding="async"
                              />
                            </button>
                            <button
                              class="stamp-delete"
                              aria-label=${`Delete recent stamp ${d+1}`}
                              title=${`Delete recent stamp ${d+1}`}
                              @click=${f=>this._deleteStamp(l,f)}
                              ?disabled=${this._stampBusy}
                            >&times;</button>
                          </div>
                        `)}
                    </div>
                  `:""}
            </div>
            <div
              class="stamp-help ${this._stampMessageError?"error":""}"
              role="status"
              aria-live="polite"
            >
              ${this._stampMessage||(o?"Click the canvas to place the selected stamp. Enter accepts; Escape cancels.":"Choose a recent stamp or upload an image, then click the canvas.")}
            </div>
          `:""}

      ${this.ctx.saving?g`
            <div class="saving-indicator" role="img" aria-label="Saving">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56"></path>
              </svg>
            </div>
          `:""}

      <dialog class="new-project-dialog" @keydown=${this._onNewProjectKeydown}>
        <p class="dialog-title">New Project</p>
        <div class="dialog-field">
          <label>Name</label>
          <input
            type="text"
            class="new-project-name-input"
            .value=${this._newProjectName}
            @input=${l=>{this._newProjectName=l.target.value}}
          />
        </div>
        <div class="dialog-field">
          <label>Canvas Size</label>
          <div class="dialog-presets">
            ${Aa.map(l=>g`
              <button
                class="dialog-preset-btn ${String(l.width)===this._newProjectWidth&&String(l.height)===this._newProjectHeight?"active":""}"
                @click=${()=>this._selectNewProjectPreset(l)}
              >${l.label}</button>
            `)}
          </div>
          <div class="dialog-size-row">
            <input
              class="dialog-size-input"
              type="number"
              min="1"
              max="8192"
              .value=${this._newProjectWidth}
              @input=${l=>{this._newProjectWidth=l.target.value}}
            />
            <span>\u00d7</span>
            <input
              class="dialog-size-input"
              type="number"
              min="1"
              max="8192"
              .value=${this._newProjectHeight}
              @input=${l=>{this._newProjectHeight=l.target.value}}
            />
          </div>
        </div>
        <div class="dialog-actions">
          <button class="dialog-cancel-btn" @click=${this._cancelNewProject}>Cancel</button>
          <button class="dialog-create-btn" @click=${this._confirmNewProject}>Create</button>
        </div>
      </dialog>
    `}};j.styles=bt`
    :host {
      display: flex;
      align-items: center;
      background: #333;
      /* Right padding keeps controls clear of the saving indicator */
      padding: 0.375rem 2.75rem 0.375rem 1rem;
      column-gap: 1rem;
      row-gap: 0.25rem;
      color: #ddd;
      font-family: system-ui, sans-serif;
      font-size: 0.8125rem;
      flex-wrap: wrap;
      min-height: 2.75rem;
      position: relative;
    }

    .section {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }

    label {
      color: #aaa;
      white-space: nowrap;
    }

    .color-grid {
      display: flex;
      flex-wrap: wrap;
      gap: 0.1875rem;
      max-width: 15rem;
    }

    .color-swatch {
      width: 1.5rem;
      height: 1.5rem;
      border-radius: 0.1875rem;
      border: 0.125rem solid transparent;
      cursor: pointer;
      padding: 0;
      box-sizing: border-box;
    }

    .color-swatch:hover {
      border-color: #888;
    }

    .color-swatch.active {
      border-color: #5b8cf7;
    }

    input[type="color"] {
      width: 1.75rem;
      height: 1.75rem;
      border: none;
      border-radius: 0.25rem;
      padding: 0;
      cursor: pointer;
      background: none;
    }

    input[type="range"] {
      width: 6.25rem;
      accent-color: #5b8cf7;
    }

    .size-value {
      min-width: 1.5rem;
      text-align: center;
    }

    .stamp-size-input {
      width: 4.5rem;
      min-height: 2rem;
      box-sizing: border-box;
      border: 1px solid #555;
      border-radius: 0.25rem;
      background: #222;
      color: #ddd;
      padding: 0.25rem 0.375rem;
    }

    .checkbox-label {
      display: flex;
      align-items: center;
      gap: 0.25rem;
      cursor: pointer;
      color: #aaa;
    }

    .checkbox-label input {
      accent-color: #5b8cf7;
    }

    .stamp-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      background: #5b8cf7;
      color: white;
      border: none;
      border-radius: 6px;
      cursor: pointer;
      padding: 6px;
      flex-shrink: 0;
    }

    .stamp-btn svg {
      width: 15px;
      height: 15px;
    }

    .stamp-btn:hover {
      background: #4a7be6;
    }

    .stamp-btn:disabled,
    .stamp-thumb:disabled,
    .stamp-delete:disabled {
      opacity: 0.55;
      cursor: wait;
    }

    .separator {
      width: 0.0625rem;
      height: 1.5rem;
      background: #555;
    }

    .stamp-row {
      display: flex;
      gap: 0.25rem;
      overflow-x: auto;
      overflow-y: hidden;
      padding: 0.125rem 0;
      align-items: center;
      scrollbar-width: none;
    }

    .stamp-row::-webkit-scrollbar {
      display: none;
    }

    .stamp-thumb-wrap {
      position: relative;
      flex-shrink: 0;
      overflow: hidden;
      border-radius: 0.25rem;
    }

    .stamp-thumb {
      width: 2.75rem;
      height: 2.75rem;
      border-radius: 0.25rem;
      border: 0.125rem solid transparent;
      object-fit: contain;
      background: #222;
      cursor: pointer;
      display: block;
      padding: 0;
      box-sizing: border-box;
    }

    .stamp-thumb img {
      width: 100%;
      height: 100%;
      object-fit: contain;
      display: block;
    }

    .stamp-thumb:hover {
      border-color: #888;
    }

    .stamp-thumb.active {
      border-color: #5b8cf7;
    }

    .stamp-delete {
      position: absolute;
      top: -0.125rem;
      right: -0.125rem;
      width: 1.5rem;
      height: 1.5rem;
      border-radius: 50%;
      background: #444;
      color: #fff;
      border: none;
      font-size: 0.875rem;
      line-height: 1.5rem;
      text-align: center;
      cursor: pointer;
      padding: 0;
      z-index: 1;
    }

    @media (hover: hover) and (pointer: fine) {
      .stamp-delete {
        opacity: 0;
        pointer-events: none;
      }

      .stamp-thumb-wrap:hover .stamp-delete,
      .stamp-thumb-wrap:focus-within .stamp-delete {
        opacity: 1;
        pointer-events: auto;
      }
    }

    @media (hover: none), (pointer: coarse) {
      .stamp-btn {
        width: 2.75rem;
        height: 2.75rem;
      }

      .stamp-thumb-wrap {
        display: flex;
        gap: 0.125rem;
        overflow: visible;
      }

      .stamp-delete {
        position: static;
        width: 2.75rem;
        height: 2.75rem;
        border-radius: 0.25rem;
        line-height: 2.75rem;
      }
    }

    .stamp-delete:hover {
      background: #e55;
    }

    .stamp-line {
      flex-basis: 100%;
      display: flex;
      align-items: center;
      gap: 0.5rem;
      margin-left: -0.5rem;
    }

    .stamp-help {
      flex-basis: 100%;
      color: #aaa;
      font-size: 0.75rem;
      margin-left: -0.5rem;
    }

    .stamp-help.error {
      color: #ff8a8a;
    }

    .project-section {
      position: relative;
    }

    .project-dropdown-wrap {
      position: relative;
    }

    .project-name-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.5rem;
      cursor: pointer;
      font-size: 0.8125rem;
      display: flex;
      align-items: center;
      gap: 0.25rem;
      max-width: 12rem;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .project-name-btn:hover {
      background: #555;
    }

    .dropdown-arrow {
      font-size: 0.625rem;
      opacity: 0.7;
    }

    .project-dropdown {
      position: absolute;
      top: 100%;
      left: 0;
      margin-top: 0.25rem;
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      min-width: 14rem;
      max-height: 20rem;
      overflow-y: auto;
      z-index: 100;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
      padding: 0.25rem 0;
    }

    .project-item {
      display: flex;
      align-items: center;
      padding: 0.375rem 0.5rem;
      gap: 0.25rem;
    }

    .project-item.active {
      background: #4a4a4a;
    }

    .project-item-name {
      flex: 1;
      cursor: pointer;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      padding: 0.125rem 0;
    }

    .project-item-name:hover {
      color: #fff;
    }

    .project-item-action {
      background: none;
      border: none;
      color: #bbb;
      cursor: pointer;
      padding: 0;
      border-radius: 0.25rem;
      line-height: 0;
      width: 1.75rem;
      height: 1.75rem;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      flex-shrink: 0;
    }

    .project-item-action:hover,
    .project-item-action:focus-visible {
      color: #fff;
      background: #555;
    }

    .project-item-action.delete:hover,
    .project-item-action.delete:focus-visible {
      color: #ff8080;
      background: #5a3434;
    }

    .project-rename-input {
      flex: 1;
      background: #2a2a2a;
      border: 1px solid #5b8cf7;
      border-radius: 0.1875rem;
      color: #ddd;
      padding: 0.125rem 0.25rem;
      font-size: 0.8125rem;
      outline: 2px solid transparent;
    }

    .project-dropdown-divider {
      height: 1px;
      background: #555;
      margin: 0.25rem 0;
    }

    .project-new-btn {
      display: block;
      width: 100%;
      text-align: left;
      background: none;
      border: none;
      color: #5b8cf7;
      cursor: pointer;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      height: auto;
      border-radius: 0;
    }

    .project-new-btn:hover {
      background: #4a4a4a;
      color: #7aa8ff;
    }

    .saving-indicator {
      position: absolute;
      top: 0.75rem;
      right: 1rem;
      color: #888;
      width: 1.25rem;
      height: 1.25rem;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .saving-indicator svg {
      width: 100%;
      height: 100%;
      animation: spin 1s linear infinite;
    }

    @keyframes spin {
      100% { transform: rotate(360deg); }
    }

    dialog {
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.5rem;
      color: #ddd;
      padding: 1.25rem;
      min-width: 18rem;
      box-shadow: 0 8px 24px rgba(0,0,0,0.5);
      font-family: system-ui, sans-serif;
      font-size: 0.8125rem;
    }

    dialog::backdrop {
      background: rgba(0,0,0,0.5);
    }

    .dialog-title {
      font-size: 1rem;
      font-weight: 600;
      margin: 0 0 1rem 0;
    }

    .dialog-field {
      display: flex;
      flex-direction: column;
      gap: 0.25rem;
      margin-bottom: 0.75rem;
    }

    .dialog-field label {
      color: #aaa;
      font-size: 0.75rem;
    }

    .dialog-field input[type="text"] {
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      outline: 2px solid transparent;
    }

    .dialog-field input[type="text"]:focus {
      border-color: #5b8cf7;
    }

    .dialog-presets {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
      margin-bottom: 0.5rem;
    }

    .dialog-preset-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.5rem;
      cursor: pointer;
      font-size: 0.75rem;
      height: auto;
    }

    .dialog-preset-btn:hover {
      background: #555;
    }

    .dialog-preset-btn.active {
      border-color: #5b8cf7;
      color: #5b8cf7;
    }

    .dialog-size-row {
      display: flex;
      align-items: center;
      gap: 0.375rem;
    }

    .dialog-size-input {
      width: 5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      text-align: center;
      outline: 2px solid transparent;
    }

    .dialog-size-input:focus {
      border-color: #5b8cf7;
    }

    .dialog-actions {
      display: flex;
      justify-content: flex-end;
      gap: 0.5rem;
      margin-top: 1rem;
    }

    .dialog-cancel-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.375rem 0.75rem;
      cursor: pointer;
      font-size: 0.8125rem;
    }

    .dialog-cancel-btn:hover {
      background: #555;
    }

    .dialog-create-btn {
      background: #5b8cf7;
      color: white;
      border: none;
      border-radius: 0.25rem;
      padding: 0.375rem 0.75rem;
      cursor: pointer;
      font-size: 0.8125rem;
    }

    .dialog-create-btn:hover {
      background: #4a7be6;
    }

    .font-select {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.375rem;
      font-size: 0.8125rem;
      cursor: pointer;
    }

    .font-size-input {
      width: 3.5rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.375rem;
      font-size: 0.8125rem;
      text-align: center;
    }

    .font-toggle {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.75rem;
      height: 1.75rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      cursor: pointer;
      font-size: 0.8125rem;
      padding: 0;
    }

    .font-toggle.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    /* Brush preset dropdown */
    .brush-dropdown-wrap {
      position: relative;
      width: 100%;
    }

    .brush-dropdown-btn {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      width: 100%;
      padding: 0.25rem 0.5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      cursor: pointer;
      color: #ddd;
      font-size: 0.8125rem;
    }

    .brush-dropdown-btn:hover {
      border-color: #888;
    }

    .brush-dropdown-btn img {
      width: 80px;
      height: 24px;
      border-radius: 0.125rem;
      object-fit: cover;
    }

    .brush-dropdown-btn .chevron {
      margin-left: auto;
      font-size: 0.625rem;
      color: #888;
    }

    .brush-dropdown-panel {
      position: absolute;
      top: 100%;
      left: 0;
      right: 0;
      z-index: 50;
      background: #333;
      border: 1px solid #555;
      border-radius: 0.375rem;
      margin-top: 0.125rem;
      max-height: 300px;
      overflow-y: auto;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
    }

    .brush-dropdown-item {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      width: 100%;
      padding: 0.375rem 0.5rem;
      background: none;
      border: none;
      cursor: pointer;
      color: #ccc;
      font-size: 0.8125rem;
      text-align: left;
    }

    .brush-dropdown-item:hover {
      background: #444;
    }

    .brush-dropdown-item.active {
      background: #3a3a4a;
      color: #5b8cf7;
    }

    .brush-dropdown-item img {
      width: 120px;
      height: 36px;
      border-radius: 0.25rem;
      object-fit: cover;
      overflow: hidden;
    }

    /* Pill-button shape selector */
    .pill-row {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
    }

    .pill-btn {
      background: #444;
      color: #bbb;
      border: 1px solid #555;
      border-radius: 1rem;
      padding: 0.15rem 0.5rem;
      cursor: pointer;
      font-size: 0.75rem;
      white-space: nowrap;
      height: auto;
    }

    .pill-btn:hover {
      background: #555;
      color: #ddd;
    }

    .pill-btn.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    .shape-picker {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
    }

    .shape-option {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 2rem;
      height: 2rem;
      padding: 0.35rem;
      border: 1px solid #555;
      border-radius: 0.375rem;
      background: #444;
      color: #bbb;
      cursor: pointer;
    }

    .shape-option:hover {
      background: #555;
      color: #fff;
    }

    .shape-option.active {
      background: #5b8cf7;
      border-color: #77a1ff;
      color: #fff;
    }

    .shape-option svg {
      width: 1.125rem;
      height: 1.125rem;
    }

    /* Dimmed control */

    /* Transform numeric panel */
    .transform-section {
      display: flex;
      flex-direction: column;
      gap: 0.2rem;
    }

    .transform-section > label {
      font-size: 0.6875rem;
      color: #888;
      white-space: nowrap;
    }

    .transform-row {
      display: flex;
      align-items: center;
      gap: 0.25rem;
    }

    .transform-input {
      width: 4rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.2rem 0.3rem;
      font-size: 0.75rem;
      text-align: center;
      outline: 2px solid transparent;
      -moz-appearance: textfield;
    }

    .transform-input::-webkit-inner-spin-button,
    .transform-input::-webkit-outer-spin-button {
      -webkit-appearance: none;
      margin: 0;
    }

    .transform-input:focus {
      border-color: #5b8cf7;
    }

    .transform-suffix {
      color: #888;
      font-size: 0.75rem;
    }

    .flip-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.75rem;
      height: 1.75rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      cursor: pointer;
      font-size: 0.75rem;
      padding: 0;
    }

    .flip-btn:hover {
      background: #555;
    }

    .flip-btn.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    .aspect-lock-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.25rem;
      height: 1.25rem;
      background: none;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #888;
      cursor: pointer;
      padding: 0;
      font-size: 0.6875rem;
      flex-shrink: 0;
    }

    .aspect-lock-btn:hover {
      border-color: #888;
      color: #ddd;
    }

    .aspect-lock-btn.active {
      border-color: #5b8cf7;
      color: #5b8cf7;
    }

    /* ── Desktop settings panels (color, brush) ── */
    .panel-wrap {
      position: relative;
    }

    .panel-trigger {
      display: flex;
      align-items: center;
      gap: 0.375rem;
      height: 2rem;
      padding: 0 0.5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      color: #ddd;
      cursor: pointer;
      font-size: 0.8125rem;
      white-space: nowrap;
    }

    .panel-trigger:hover,
    .panel-trigger.open {
      border-color: #888;
    }

    .panel-trigger:focus-visible,
    .advanced-toggle:focus-visible {
      outline: 2px solid #5b8cf7;
      outline-offset: 1px;
    }

    .panel-trigger .chevron {
      font-size: 0.625rem;
      color: #888;
    }

    .color-chip {
      width: 1.25rem;
      height: 1.25rem;
      border-radius: 0.1875rem;
      box-shadow: inset 0 0 0 1px rgba(255,255,255,0.25);
    }

    .settings-panel {
      position: absolute;
      top: 100%;
      left: 0;
      margin-top: 0.25rem;
      z-index: 100;
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.5rem;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
      padding: 0.75rem;
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      gap: 0.5rem;
    }

    .color-panel .color-grid {
      /* Nine swatches per row, matching the two-row palette. */
      width: calc(9 * 1.5rem + 8 * 0.1875rem);
      max-width: none;
    }

    .custom-color {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      cursor: pointer;
    }

    .custom-color:hover {
      color: #ddd;
    }

    .brush-panel {
      width: 20rem;
      max-height: calc(100vh - 6rem);
      overflow-y: auto;
      overscroll-behavior: contain;
    }

    .panel-heading {
      color: #999;
      font-size: 0.6875rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      margin-top: 0.25rem;
    }

    .panel-heading:first-child {
      margin-top: 0;
    }

    /* Slider rows line up as label | slider | value columns. */
    .brush-panel .section {
      display: grid;
      grid-template-columns: 5.5rem minmax(0, 1fr) 3rem;
      align-items: center;
      gap: 0.5rem;
    }

    .brush-panel .section > input[type="range"] {
      width: 100%;
    }

    .brush-panel .section > .size-value {
      text-align: right;
    }

    .brush-panel .section > .checkbox-label {
      grid-column: 1 / -1;
    }

    .brush-panel .section > select {
      grid-column: 2 / -1;
      justify-self: start;
    }

    .brush-panel .section > .pill-row {
      grid-column: 2 / -1;
    }

    .advanced-group {
      flex-wrap: wrap;
      gap: 0.5rem;
    }

    .brush-panel .advanced-group {
      display: flex;
      flex-direction: column;
      align-items: stretch;
      gap: 0.5rem;
    }

    .advanced-toggle {
      background: none;
      border: none;
      padding: 0.125rem 0;
      color: #aaa;
      cursor: pointer;
      font-size: 0.8125rem;
      text-align: left;
      border-radius: 0.25rem;
    }

    .advanced-toggle:hover {
      color: #ddd;
    }

    .brush-panel .advanced-toggle {
      grid-column: 1 / -1;
      color: #999;
      font-size: 0.6875rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      margin-top: 0.25rem;
    }

    /* Narrower desktop windows: keep the bar on one row. */
    @media (max-width: 1180px) {
      :host(:not([mobile])) {
        column-gap: 0.75rem;
      }

      :host(:not([mobile])) input[type="range"] {
        width: 5rem;
      }

      :host(:not([mobile])) .brush-panel input[type="range"] {
        width: 100%;
      }

      :host(:not([mobile])) .brush-trigger-label {
        display: none;
      }
    }

    .document-size {
      color: #aaa;
      font-size: 0.75rem;
      white-space: nowrap;
    }

    /* ── Inside mobile popover ─────────────────── */
    :host([mobile]) {
      flex-direction: column;
      align-items: flex-start;
      padding: 0;
      min-height: 0;
      background: transparent;
      touch-action: manipulation;
    }

    :host([mobile]) .section {
      width: 100%;
      min-width: 0;
    }

    :host([mobile]) .separator {
      display: none;
    }

    :host([mobile]) input[type="range"] {
      flex: 1;
      min-width: 0;
      width: 0;
    }

    :host([mobile]) .stamp-size-input {
      min-height: 2.75rem;
    }
  `;Y([M()],j.prototype,"_aspectLock",2);Y([M()],j.prototype,"_recentStamps",2);Y([M()],j.prototype,"_stampBusy",2);Y([M()],j.prototype,"_stampMessage",2);Y([M()],j.prototype,"_stampMessageError",2);Y([M()],j.prototype,"_projectDropdownOpen",2);Y([M()],j.prototype,"_advancedOpen",2);Y([M()],j.prototype,"_brushDropdownOpen",2);Y([M()],j.prototype,"_openPanel",2);Y([M()],j.prototype,"_renamingProjectId",2);Y([M()],j.prototype,"_newProjectName",2);Y([M()],j.prototype,"_newProjectWidth",2);Y([M()],j.prototype,"_newProjectHeight",2);j=Y([yt("tool-settings")],j);var ja=Object.defineProperty,Ha=Object.getOwnPropertyDescriptor,Ue=(t,e,s,i)=>{for(var a=i>1?void 0:i?Ha(e,s):e,o=t.length-1,r;o>=0;o--)(r=t[o])&&(a=(i?r(e,s,a):r(a))||a);return i&&a&&ja(e,s,a),a};const ms=typeof navigator<"u"&&/Mac|iPhone|iPad/.test(navigator.platform)?"⌘":"Ctrl+",ae=[..._e],St=[["select","move","crop","hand"],["pencil","eraser"],ae,["fill","stamp","text","eyedropper"]],Ba=[{hex:"#000000",name:"Black"},{hex:"#ffffff",name:"White"},{hex:"#ff3b30",name:"Red"},{hex:"#ff9500",name:"Orange"},{hex:"#ffcc00",name:"Yellow"},{hex:"#34c759",name:"Green"},{hex:"#00c7be",name:"Teal"},{hex:"#007aff",name:"Blue"},{hex:"#5856d6",name:"Indigo"},{hex:"#af52de",name:"Purple"},{hex:"#ff2d55",name:"Pink"},{hex:"#a2845e",name:"Brown"}],ke=[{label:"S",value:4},{label:"M",value:16},{label:"L",value:40}];let Ft=class extends F{constructor(){super(...arguments),this._popoverGroup=null,this._isFullscreen=!1,this._lastToolPerGroup=new Map,this._ctx=new gt(this,{context:Et,subscribe:!0}),this._onFullscreenChange=()=>{this._isFullscreen=!!document.fullscreenElement},this._toggleFullscreen=()=>{document.fullscreenElement?document.exitFullscreen():document.documentElement.requestFullscreen(),this._closePopover()}}get ctx(){return this._ctx.value}get _saveLabel(){return this.ctx.embedded?"Save":"Export as PNG"}connectedCallback(){super.connectedCallback(),document.addEventListener("fullscreenchange",this._onFullscreenChange)}disconnectedCallback(){super.disconnectedCallback(),document.removeEventListener("fullscreenchange",this._onFullscreenChange)}willUpdate(){this.toggleAttribute("mobile",this.ctx?.isMobile??!1),this.toggleAttribute("child-mode",this.ctx?.state?.childMode??!1);const t=this.ctx?.state?.activeTool;if(t){const e=St.findIndex(s=>s.includes(t));e!==-1&&this._lastToolPerGroup.set(e,t)}this.ctx?.state?.layersPanelOpen&&this._popoverGroup!==null&&(this._popoverGroup=null)}_selectTool(t){this.ctx.setTool(t)}render(){if(!this._ctx.value)return g``;const{activeTool:t}=this.ctx.state;return this.ctx.isMobile?this._renderMobile(t):g`
      ${St.map((e,s)=>g`
          ${s>0?g`<div class="separator"></div>`:""}
          <div class="group">
            ${e===ae?g`
                <button
                  class=${ht(t)?"active":""}
                  title="Shapes (U)"
                  aria-label="Shapes"
                  aria-pressed=${ht(t)}
                  @click=${()=>this._selectTool(ht(t)?t:this._lastToolPerGroup.get(s)??_e[0])}
                >
                  ${rs}
                </button>
              `:e.map(i=>g`
                <button
                  class=${t===i?"active":""}
                  title=${`${nt[i]} (${We[i]})`}
                  aria-label=${nt[i]}
                  aria-pressed=${t===i}
                  @click=${()=>this._selectTool(i)}
                >
                  ${jt[i]}
                </button>
              `)}
          </div>
        `)}

      <div class="action-group">
        <div class="separator"></div>
        <button
          title=${`Undo (${ms}Z)`}
          aria-label="Undo"
          ?disabled=${!this.ctx.canUndo}
          @click=${()=>this.ctx.undo()}
        >
          ${W.undo}
        </button>
        <button
          title=${`Redo (${ms}Shift+Z)`}
          aria-label="Redo"
          ?disabled=${!this.ctx.canRedo}
          @click=${()=>this.ctx.redo()}
        >
          ${W.redo}
        </button>
        <button title=${this._saveLabel} aria-label=${this._saveLabel} @click=${()=>this.ctx.saveCanvas()}>
          ${W.save}
        </button>
        <button title="Clear layer" aria-label="Clear layer" @click=${()=>this.ctx.clearCanvas()}>
          ${W.clear}
        </button>
      </div>
    `}_closestChildSize(t){let e=ke[0].value,s=Math.abs(t-e);for(const i of ke){const a=Math.abs(t-i.value);a<s&&(s=a,e=i.value)}return e}_confirmClearCanvas(){confirm("Clear the whole drawing?")&&this.ctx.clearCanvas()}_renderChildMode(t){const e=this.ctx.state.strokeColor,s=this.ctx.state.brush.size,i=this._closestChildSize(s);return g`
      <div class="child-bar">
        <div class="child-colors">
          ${Ba.map(a=>g`
            <button
              class="child-color-btn ${e===a.hex?"active":""}"
              style="background:${a.hex}${a.hex==="#ffffff"?";box-shadow:inset 0 0 0 1px #666":""}"
              aria-label=${a.name}
              @click=${()=>this.ctx.setStrokeColor(a.hex)}
            ></button>
          `)}
          <input
            type="color"
            class="child-color-picker"
            .value=${e}
            @input=${a=>this.ctx.setStrokeColor(a.target.value)}
            title="Pick color"
          />
        </div>

        <div class="child-tools-row">
          <button
            class="child-tool-btn"
            title="Undo"
            ?disabled=${!this.ctx.canUndo}
            @click=${()=>this.ctx.undo()}
          >${W.undo}</button>
          <button
            class="child-tool-btn"
            title="Redo"
            ?disabled=${!this.ctx.canRedo}
            @click=${()=>this.ctx.redo()}
          >${W.redo}</button>

          <div class="child-sep"></div>

          ${Bs.map(a=>g`
            <button
              class="child-tool-btn ${t===a?"active":""}"
              title=${nt[a]}
              @click=${()=>this._selectTool(a)}
            >${jt[a]}</button>
          `)}

          <div class="child-sep"></div>

          ${ke.map(a=>g`
            <button
              class="child-size-btn ${i===a.value?"active":""}"
              title="${a.label} brush"
              @click=${()=>this.ctx.setBrushSize(a.value)}
            >${a.label}</button>
          `)}

          <div class="child-sep"></div>

          <button
            class="child-tool-btn"
            title=${this._saveLabel}
            aria-label=${this._saveLabel}
            @click=${()=>this.ctx.saveCanvas()}
          >${W.save}</button>
          <button
            class="child-tool-btn"
            title="Clear canvas"
            @click=${()=>this._confirmClearCanvas()}
          >${W.clear}</button>
          <button
            class="child-tool-btn"
            title="Exit Child Mode"
            @click=${()=>this.ctx.setChildMode(!1)}
          >${W.exitChildMode}</button>
        </div>
      </div>
    `}_renderMobile(t){return this.ctx.state.childMode?this._renderChildMode(t):g`
      <!-- Undo/Redo at left -->
      <button
        title="Undo"
        ?disabled=${!this.ctx.canUndo}
        @click=${()=>this.ctx.undo()}
      >${W.undo}</button>
      <button
        title="Redo"
        ?disabled=${!this.ctx.canRedo}
        @click=${()=>this.ctx.redo()}
      >${W.redo}</button>

      <div class="separator"></div>

      <!-- Tool groups: show one representative button per group -->
      ${St.map((e,s)=>{const a=e.find(n=>n===t)??e[0],o=e.includes(t),r=e===ae;return g`
          <button
            class=${o?"active":""}
            title=${r?"Shapes":nt[a]}
            aria-label=${r?"Shapes":nt[a]}
            aria-expanded=${this._popoverGroup===s}
            aria-controls="mobile-tool-popover"
            @click=${()=>this._onMobileToolTap(e,s)}
          >${r?rs:jt[a]}</button>
        `})}

      <div class="separator"></div>

      <!-- Layers button -->
      <button
        title="Layers"
        @click=${()=>{this._closePopover(),this.ctx.toggleLayersPanel()}}
      >${g`<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>`}</button>

      <!-- More actions -->
      <button
        title="More"
        @click=${()=>this._onMobileMoreTap()}
      ><svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/></svg></button>

      <!-- Popover -->
      ${this._popoverGroup!==null?g`
        <div class="popover-backdrop" @click=${()=>this._closePopover()}></div>
        <div class="popover" id="mobile-tool-popover">
          ${this._renderPopoverContent(t)}
        </div>
      `:""}
    `}_onMobileToolTap(t,e){const{activeTool:s}=this.ctx.state;if(t.includes(s)){const a=this._popoverGroup===e?null:e;this._popoverGroup=a,a!==null&&this.ctx.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}else this.ctx.setTool(this._lastToolPerGroup.get(e)??t[0]),this._popoverGroup=null}_onMobileMoreTap(){const t=this._popoverGroup===-1?null:-1;this._popoverGroup=t,t!==null&&this.ctx.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}_closePopover(){this._popoverGroup=null}_stopCropActionKeydown(t){t.key==="Enter"&&t.stopPropagation()}_dispatchCropShortcut(t){this.dispatchEvent(new KeyboardEvent("keydown",{key:t,bubbles:!0,composed:!0})),this._closePopover()}_renderPopoverContent(t){if(this._popoverGroup===-1)return g`
        <button class="menu-btn" @click=${()=>{const s=St.findIndex(i=>i.includes(t));s!==-1&&this._onMobileToolTap(St[s],s)}}>Tool settings</button>
        <div class="popover-divider"></div>
        ${this.ctx.embedded?"":g`
        <span class="popover-label">Projects</span>
        <div class="project-list">
          ${this.ctx.projectList.map(s=>g`
            <button
              class="project-item ${s.id===this.ctx.currentProject?.id?"current":""}"
              @click=${()=>{this.ctx.switchProject(s.id),this._closePopover()}}
            >
              <span class="check">${s.id===this.ctx.currentProject?.id?"✓":""}</span>
              ${s.name}
            </button>
          `)}
        </div>
        <button class="menu-btn" @click=${()=>{this.ctx.createProject("Untitled",800,600),this._closePopover()}}>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          New Project
        </button>
        <div class="popover-divider"></div>
        `}
        <button class="menu-btn" @click=${()=>{this.ctx.saveCanvas(),this._closePopover()}}>${W.save} ${this._saveLabel}</button>
        <button class="menu-btn" @click=${()=>{this.ctx.clearCanvas(),this._closePopover()}}>${W.clear} Clear layer</button>
        <div class="popover-divider"></div>
        <button class="menu-btn" @click=${this._toggleFullscreen}>
          ${this._isFullscreen?g`<svg viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M5 2v3H2M14 5h-3V2M11 14v-3h3M2 11h3v3"/></svg>`:g`<svg viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4"/></svg>`}
          ${this._isFullscreen?"Exit Fullscreen":"Fullscreen"}
        </button>
        <div class="popover-divider"></div>
        <button class="menu-btn" @click=${()=>{this.ctx.setChildMode(!0),this._closePopover()}}>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
          Child Mode
        </button>
      `;const e=St[this._popoverGroup];return e?g`
      ${e.length>1&&e!==ae?g`
        <div class="sub-tools">
          ${e.map(s=>g`
            <button
              class=${t===s?"active":""}
              title=${nt[s]}
              @click=${()=>{this.ctx.setTool(s),this._lastToolPerGroup.set(this._popoverGroup,s)}}
            >${jt[s]}</button>
          `)}
        </div>
      `:""}
      ${t==="crop"?g`
        <button
          class="menu-btn"
          @keydown=${this._stopCropActionKeydown}
          @click=${()=>this._dispatchCropShortcut("Enter")}
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
          Apply crop
        </button>
        <button
          class="menu-btn"
          @keydown=${this._stopCropActionKeydown}
          @click=${()=>this._dispatchCropShortcut("Escape")}
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
          Cancel crop
        </button>
        <div class="popover-divider"></div>
      `:""}
      <tool-settings></tool-settings>
    `:g``}};Ft.styles=bt`
    :host {
      display: flex;
      flex-direction: column;
      background: #2c2c2c;
      padding: 8px;
      gap: 4px;
      width: 60px;
      box-sizing: border-box;
      overflow-x: hidden;
      overflow-y: auto;
      scrollbar-width: thin;
      scrollbar-color: #555 transparent;
    }

    .group {
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    .separator {
      height: 1px;
      background: #555;
      margin: 4px 2px;
    }

    button {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      padding: 10px;
      transition: all 0.15s ease;
    }

    button:hover {
      background: #444;
      color: #fff;
    }

    button.active {
      background: #5b8cf7;
      color: #fff;
    }

    button:disabled {
      opacity: 0.3;
      cursor: default;
    }

    button:disabled:hover {
      background: transparent;
      color: #bbb;
    }

    button svg {
      width: 20px;
      height: 20px;
    }

    .action-group {
      margin-top: auto;
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    /* ── Mobile bottom bar ─────────────────────── */
    :host([mobile]) {
      flex-direction: row;
      width: 100%;
      height: 48px;
      padding: 4px 8px;
      padding-bottom: calc(4px + env(safe-area-inset-bottom));
      align-items: center;
      justify-content: space-between;
      overflow: visible;
      border-top: 1px solid #444;
      touch-action: manipulation;
    }

    :host([mobile][child-mode]) {
      height: auto;
      padding: 8px 8px;
      padding-bottom: calc(8px + env(safe-area-inset-bottom));
      justify-content: center;
    }

    :host([mobile]) .group {
      flex-direction: row;
    }

    :host([mobile]) .separator {
      width: 1px;
      height: 24px;
      margin: 0 4px;
    }

    :host([mobile]) .action-group {
      flex-direction: row;
      margin-top: 0;
      margin-left: auto;
    }

    .popover {
      display: none;
    }

    :host([mobile]) .popover {
      display: flex;
      flex-direction: column;
      position: absolute;
      bottom: calc(52px + env(safe-area-inset-bottom));
      left: 8px;
      right: 8px;
      max-height: calc(100dvh - 72px - env(safe-area-inset-bottom));
      box-sizing: border-box;
      overflow-y: auto;
      overscroll-behavior: contain;
      background: #2c2c2c;
      border: 1px solid #555;
      border-radius: 12px;
      padding: 8px;
      gap: 8px;
      z-index: 100;
      touch-action: manipulation;
      box-shadow: 0 -4px 16px rgba(0,0,0,0.4);
    }

    .popover > * {
      flex-shrink: 0;
    }

    .popover-backdrop {
      display: none;
    }

    :host([mobile]) .popover-backdrop {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 99;
    }

    .popover .sub-tools {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      padding-bottom: 8px;
      border-bottom: 1px solid #444;
    }

    .popover-divider {
      height: 1px;
      background: #444;
    }

    .popover-label {
      font-size: 0.7rem;
      color: #888;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      padding: 0 4px;
    }

    .popover .project-list {
      display: flex;
      flex-direction: column;
      gap: 2px;
      max-height: 40vh;
      overflow-y: auto;
    }

    .popover .project-item {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 6px;
      width: 100%;
      padding: 8px 10px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      font-size: 0.85rem;
      text-align: left;
      cursor: pointer;
    }

    .popover .project-item:hover {
      background: #444;
    }

    .popover .project-item.current {
      background: #3a3a3a;
      color: #fff;
    }

    .popover .project-item .check {
      width: 16px;
      flex-shrink: 0;
      color: #5b8cf7;
    }

    .popover button.menu-btn {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 8px;
      width: 100%;
      height: auto;
      padding: 10px;
      font-size: 0.85rem;
      text-align: left;
      border-radius: 6px;
    }

    .popover button.menu-btn svg {
      flex-shrink: 0;
    }

    /* ── Child Mode ───────────────────────────── */
    .child-bar {
      display: flex;
      flex-direction: column;
      width: 100%;
      gap: 6px;
    }

    .child-colors {
      display: flex;
      justify-content: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .child-color-btn {
      width: 36px;
      height: 36px;
      border-radius: 50%;
      border: 3px solid transparent;
      padding: 0;
      cursor: pointer;
      transition: transform 0.15s ease, border-color 0.15s ease;
    }

    .child-color-btn:hover {
      transform: scale(1.15);
    }

    .child-color-btn.active {
      border-color: #fff;
      box-shadow: 0 0 0 2px #5b8cf7;
      transform: scale(1.15);
    }

    .child-tools-row {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 4px;
      flex-wrap: wrap;
    }

    .child-tool-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 48px;
      height: 48px;
      border: none;
      border-radius: 14px;
      background: #3a3a3a;
      color: #ccc;
      cursor: pointer;
      padding: 0;
      transition: all 0.15s ease;
    }

    .child-tool-btn svg {
      width: 22px;
      height: 22px;
    }

    .child-tool-btn:hover {
      background: #555;
      color: #fff;
    }

    .child-tool-btn.active {
      background: #5b8cf7;
      color: #fff;
    }

    .child-tool-btn:disabled {
      opacity: 0.3;
      cursor: default;
    }

    .child-size-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 40px;
      height: 48px;
      border: none;
      border-radius: 14px;
      background: #3a3a3a;
      color: #ccc;
      font-size: 0.85rem;
      font-weight: 700;
      cursor: pointer;
      padding: 0;
      transition: all 0.15s ease;
    }

    .child-size-btn:hover {
      background: #555;
      color: #fff;
    }

    .child-size-btn.active {
      background: #ff9500;
      color: #fff;
    }

    .child-sep {
      width: 1px;
      height: 32px;
      background: #555;
      margin: 0 2px;
      flex-shrink: 0;
    }

    .child-color-picker {
      width: 36px;
      height: 36px;
      border: none;
      border-radius: 50%;
      padding: 0;
      cursor: pointer;
      background: none;
    }
  `;Ue([M()],Ft.prototype,"_popoverGroup",2);Ue([M()],Ft.prototype,"_isFullscreen",2);Ft=Ue([yt("app-toolbar")],Ft);function Ne(t,e=0){let s=null,i=null,a=0;const o=()=>{s=null,i=null,a=performance.now(),t()};return{schedule(){if(s!==null||i!==null)return;const r=e-(performance.now()-a);r<=0?s=requestAnimationFrame(o):i=setTimeout(()=>{i=null,s=requestAnimationFrame(o)},r)},cancel(){s!==null&&cancelAnimationFrame(s),i!==null&&clearTimeout(i),s=null,i=null}}}function gs(t){return Math.max(0,Math.min(1,t))}function vs(t){return t.pointerType==="mouse"?1:t.pointerType==="pen"?Number.isFinite(t.pressure)?gs(t.pressure):.5:t.pointerType==="touch"?1:t.pressure>0&&Number.isFinite(t.pressure)?gs(t.pressure):.5}function Ya(t,e,s,i,a=32){const{width:o,height:r}=t.canvas,n=Math.floor(e),c=Math.floor(s);if(n<0||n>=o||c<0||c>=r)return!1;const h=t.getImageData(0,0,o,r),l=h.data,d=Ua(i),f=(c*o+n)*4,u=l[f],p=l[f+1],_=l[f+2],m=l[f+3];if(a===0&&u===d.r&&p===d.g&&_===d.b&&m===d.a)return!1;const v=Xa(o*r),b=oe,y=C=>Wa(l,C*4,u,p,_,m,a),x=[n,c];for(;x.length>0;){const C=x.pop(),k=x.pop(),T=C*o;if(v[T+k]===b||!y(T+k))continue;let S=k;for(;S>0&&v[T+S-1]!==b&&y(T+S-1);)S--;let E=k;for(;E<o-1&&v[T+E+1]!==b&&y(T+E+1);)E++;for(let H=S;H<=E;H++){const B=(T+H)*4;l[B]=d.r,l[B+1]=d.g,l[B+2]=d.b,l[B+3]=d.a,v[T+H]=b}C>0&&bs(x,v,b,y,o,C-1,S,E),C<r-1&&bs(x,v,b,y,o,C+1,S,E)}return t.putImageData(h,0,0),!0}function bs(t,e,s,i,a,o,r,n){const c=o*a;let h=!1;for(let l=r;l<=n;l++){const d=e[c+l]!==s&&i(c+l);d&&!h&&t.push(l,o),h=d}}function Wa(t,e,s,i,a,o,r){return Math.abs(t[e]-s)<=r&&Math.abs(t[e+1]-i)<=r&&Math.abs(t[e+2]-a)<=r&&Math.abs(t[e+3]-o)<=r}let Qt=new Uint16Array(0),oe=0;function Xa(t){return Qt.length<t&&(Qt=new Uint16Array(t),oe=0),++oe>65535&&(Qt.fill(0),oe=1),Qt}const te=new Map;let ut=null;function Ua(t){const e=te.get(t);if(e)return e;if(!ut){const n=document.createElement("canvas");n.width=1,n.height=1,ut=n.getContext("2d",{willReadFrequently:!0})}ut.clearRect(0,0,1,1),ut.fillStyle="#000",ut.fillStyle=t,ut.fillRect(0,0,1,1);const[s,i,a,o]=ut.getImageData(0,0,1,1).data,r={r:s,g:i,b:a,a:o};return te.size>64&&te.clear(),te.set(t,r),r}function Na(t,e,s,i,a,o){t.save(),t.lineWidth=1,t.setLineDash([6,6]),t.strokeStyle="#ffffff",t.lineDashOffset=0,t.strokeRect(e+.5,s+.5,i,a),t.strokeStyle="#3b82f6",t.lineDashOffset=o,t.strokeRect(e+.5,s+.5,i,a),t.restore()}const Ns=8;function Fa(t,e,s,i,a){const o=Ns/a,r=o/2;t.fillStyle="rgba(0, 0, 0, 0.5)",t.fillRect(0,0,s,e.y),t.fillRect(0,e.y+e.h,s,i-e.y-e.h),t.fillRect(0,e.y,e.x,e.h),t.fillRect(e.x+e.w,e.y,s-e.x-e.w,e.h),t.strokeStyle="#ffffff",t.lineWidth=1/a,t.setLineDash([]),t.strokeRect(e.x,e.y,e.w,e.h);const n=[{cx:e.x,cy:e.y},{cx:e.x+e.w/2,cy:e.y},{cx:e.x+e.w,cy:e.y},{cx:e.x+e.w,cy:e.y+e.h/2},{cx:e.x+e.w,cy:e.y+e.h},{cx:e.x+e.w/2,cy:e.y+e.h},{cx:e.x,cy:e.y+e.h},{cx:e.x,cy:e.y+e.h/2}];t.fillStyle="#ffffff",t.strokeStyle="#3b82f6",t.lineWidth=1/a;for(const{cx:p,cy:_}of n)t.fillRect(p-r,_-r,o,o),t.strokeRect(p-r,_-r,o,o);const c=`${Math.round(e.w)} × ${Math.round(e.h)}`,h=Math.max(10,12/a);t.font=`${h}px system-ui, sans-serif`,t.textAlign="right",t.textBaseline="top";const l=e.x+e.w,d=e.y+e.h+4/a,f=t.measureText(c),u=3/a;t.fillStyle="rgba(0, 0, 0, 0.7)",t.fillRect(l-f.width-u*2,d,f.width+u*2,h+u*2),t.fillStyle="#ffffff",t.fillText(c,l-u,d+u)}function ys(t,e,s){const a=Ns/s/2,o=[{handle:"nw",cx:t.x,cy:t.y},{handle:"n",cx:t.x+t.w/2,cy:t.y},{handle:"ne",cx:t.x+t.w,cy:t.y},{handle:"e",cx:t.x+t.w,cy:t.y+t.h/2},{handle:"se",cx:t.x+t.w,cy:t.y+t.h},{handle:"s",cx:t.x+t.w/2,cy:t.y+t.h},{handle:"sw",cx:t.x,cy:t.y+t.h},{handle:"w",cx:t.x,cy:t.y+t.h/2}];for(const{handle:r,cx:n,cy:c}of o)if(e.x>=n-a&&e.x<=n+a&&e.y>=c-a&&e.y<=c+a)return r;return e.x>=t.x&&e.x<=t.x+t.w&&e.y>=t.y&&e.y<=t.y+t.h?"move":null}function Pe(t){if(t==="free")return null;const e=t.split(":");if(e.length!==2)return null;const s=parseFloat(e[0]),i=parseFloat(e[1]);return!s||!i?null:s/i}function ws(t,e,s){const{x:i,y:a,w:o,h:r}=t,n=Math.abs(o),c=Math.abs(r),h=o>=0?1:-1,l=r>=0?1:-1;let d,f;s==="n"||s==="s"?(f=c,d=c*e):s==="e"||s==="w"||n/e>=c?(d=n,f=n/e):(f=c,d=c*e);const u=d*h,p=f*l,_=s==="nw"||s==="w"||s==="sw",m=s==="nw"||s==="n"||s==="ne",v=_?i+o-u:i,b=m?a+r-p:a;return{x:v,y:b,w:u,h:p}}const le=1.2;function he(t,e,s,i){const a=e.includes(" ")?`'${e}'`:e;return`${i?"italic ":""}${s?"bold ":""}${t}px ${a}`}function xs(t,e,s,i,a,o,r,n,c){if(!e)return;t.save(),t.font=he(a,o,r,n),t.fillStyle=c,t.textBaseline="top";const h=a*le,l=e.split(`
`);for(let d=0;d<l.length;d++)t.fillText(l[d],s,i+d*h);t.restore()}function Cs(t,e,s,i,a,o){t.save(),t.font=he(s,i,a,o),t.textBaseline="top";const r=e.split(`
`),n=s*le,c=r.map(d=>t.measureText(d).width);let h=0;for(const d of c)d>h&&(h=d);const l=Math.max(1,r.length)*n;return t.restore(),{width:h,height:l,lineWidths:c}}const Ms={size:8,hitRadius:6,shape:"square",rotationStemLength:30},Va={size:20,hitRadius:20,shape:"circle",rotationStemLength:50},qa=4,Za=3;function de(t){const e=t.width/2,s=t.height/2,i=t.skewX*Math.PI/180,a=t.skewY*Math.PI/180,o=Math.cos(t.rotation),r=Math.sin(t.rotation),n=(p,_)=>({a:p.a*_.a+p.c*_.b,b:p.b*_.a+p.d*_.b,c:p.a*_.c+p.c*_.d,d:p.b*_.c+p.d*_.d,e:p.a*_.e+p.c*_.f+p.e,f:p.b*_.e+p.d*_.f+p.f}),c=(p,_)=>({a:1,b:0,c:0,d:1,e:p,f:_}),h=()=>({a:o,b:r,c:-r,d:o,e:0,f:0}),l=()=>({a:1,b:0,c:Math.tan(i),d:1,e:0,f:0}),d=()=>({a:1,b:Math.tan(a),c:0,d:1,e:0,f:0}),f=()=>({a:t.scaleX,b:0,c:0,d:t.scaleY,e:0,f:0}),u=[c(t.x+e,t.y+s),h(),l(),d(),f(),c(-e,-s)].reduce(n,{a:1,b:0,c:0,d:1,e:0,f:0});return new DOMMatrix([u.a,u.b,u.c,u.d,u.e,u.f])}function Re(t,e){const s=de(e),i=s.a*s.d-s.b*s.c;return Math.abs(i)<1e-10?{x:t.x,y:t.y}:{x:(s.d*t.x-s.c*t.y+s.c*s.f-s.d*s.e)/i,y:(-s.b*t.x+s.a*t.y+s.b*s.e-s.a*s.f)/i}}function A(t,e){const s=de(e);return{x:s.a*t.x+s.c*t.y+s.e,y:s.b*t.x+s.d*t.y+s.f}}function Ka(t){const{width:e,height:s}=t;return[A({x:0,y:0},t),A({x:e,y:0},t),A({x:e,y:s},t),A({x:0,y:s},t)]}function Wt(t){return A({x:t.width/2,y:t.height/2},t)}function Ga(t,e){return Math.round(t/e)*e}function ks(t){const{data:e,width:s,height:i}=t;let a=s,o=-1,r=-1,n=-1;for(let c=0;c<i;c++){const h=c*s*4+3;let l=0;for(;l<s&&e[h+l*4]===0;)l++;if(l===s)continue;o<0&&(o=c),n=c,l<a&&(a=l);let d=s-1;for(;d>r&&e[h+d*4]===0;)d--;d>r&&(r=d)}return o<0?null:{x:a,y:o,w:r-a+1,h:n-o+1}}function ee(t,e){const[s,i,a,o]=Ka(t);return[{x:s.x+e.nw.x,y:s.y+e.nw.y},{x:i.x+e.ne.x,y:i.y+e.ne.y},{x:a.x+e.se.x,y:a.y+e.se.y},{x:o.x+e.sw.x,y:o.y+e.sw.y}]}function Ps(t,e,s,i,a){const[o,r,n,c]=s,[h,l,d,f]=i;for(let u=0;u<a;u++)for(let p=0;p<a;p++){const _=p/a,m=(p+1)/a,v=u/a,b=(u+1)/a,y=ot(o,r,n,c,_,v),x=ot(o,r,n,c,m,v),C=ot(o,r,n,c,_,b),k=ot(o,r,n,c,m,b),T=ot(h,l,d,f,_,v),S=ot(h,l,d,f,m,v),E=ot(h,l,d,f,_,b),H=ot(h,l,d,f,m,b);Ss(t,e,y,x,C,T,S,E),Ss(t,e,x,k,C,S,H,E)}}function ot(t,e,s,i,a,o){const r={x:t.x+(e.x-t.x)*a,y:t.y+(e.y-t.y)*a},n={x:i.x+(s.x-i.x)*a,y:i.y+(s.y-i.y)*a};return{x:r.x+(n.x-r.x)*o,y:r.y+(n.y-r.y)*o}}function Ss(t,e,s,i,a,o,r,n){const c=i.x-s.x,h=i.y-s.y,l=a.x-s.x,d=a.y-s.y,f=r.x-o.x,u=r.y-o.y,p=n.x-o.x,_=n.y-o.y,m=c*d-l*h;if(Math.abs(m)<1e-10)return;const v=1/m,b=d*v,y=-l*v,x=-h*v,C=c*v,k=b*f+x*p,T=y*f+C*p,S=b*u+x*_,E=y*u+C*_,H=o.x-k*s.x-T*s.y,B=o.y-S*s.x-E*s.y;t.save(),t.beginPath(),t.moveTo(o.x,o.y),t.lineTo(r.x,r.y),t.lineTo(n.x,n.y),t.closePath(),t.clip(),t.setTransform(k,S,T,E,H,B),t.drawImage(e,0,0),t.restore()}function Ja(t,e){return{nw:{x:0,y:0},n:{x:t/2,y:0},ne:{x:t,y:0},e:{x:t,y:e/2},se:{x:t,y:e},s:{x:t/2,y:e},sw:{x:0,y:e},w:{x:0,y:e/2}}}function Fs(t){const e=Ja(t.width,t.height),s={};for(const[i,a]of Object.entries(e))s[i]=A(a,t);return s}function Vs(t,e,s){const i=A({x:t.width/2,y:0},t),a=Wt(t),o=i.x-a.x,r=i.y-a.y,n=Math.sqrt(o*o+r*r);if(n<1)return i;const c=e.rotationStemLength/s;return{x:i.x+o/n*c,y:i.y+r/n*c}}function qs(t,e,s,i){const a=Fs(e),o=s.hitRadius/i;for(const[r,n]of Object.entries(a)){const c=t.x-n.x,h=t.y-n.y;if(c*c+h*h<=o*o)return r}return null}function Zs(t,e,s,i){const a=Vs(e,s,i),o=s.hitRadius/i,r=t.x-a.x,n=t.y-a.y;return r*r+n*n<=o*o}function Ks(t,e){const s=Re(t,e);return s.x>=0&&s.x<=e.width&&s.y>=0&&s.y<=e.height}function Qa(t,e,s,i){const a=Fs(e),o=s.size/2/i;t.save(),t.fillStyle="#ffffff",t.strokeStyle="#3b82f6",t.lineWidth=1.5/i;for(const r of Object.values(a))s.shape==="circle"?(t.beginPath(),t.arc(r.x,r.y,o,0,Math.PI*2),t.fill(),t.stroke()):(t.fillRect(r.x-o,r.y-o,o*2,o*2),t.strokeRect(r.x-o,r.y-o,o*2,o*2));t.restore()}function to(t,e,s,i){const a=A({x:e.width/2,y:0},e),o=Vs(e,s,i),r=(s.shape==="circle"?8:6)/i;t.save(),t.strokeStyle="#3b82f6",t.lineWidth=1.5/i,t.fillStyle="#ffffff",t.beginPath(),t.moveTo(a.x,a.y),t.lineTo(o.x,o.y),t.stroke(),t.beginPath(),t.arc(o.x,o.y,r,0,Math.PI*2),t.fill(),t.stroke(),t.restore()}function re(t,e,s){const i=A({x:t.width,y:0},t),a=Wt(t),o=i.x-a.x,r=i.y-a.y,n=Math.sqrt(o*o+r*r),c=e.shape==="circle",h=(c?38:30)/s,l=(c?22:12)/s,d=(c?48:28)/s,f=n>1?i.x+o/n*h:i.x+h,u=n>1?i.y+r/n*h:i.y-h;return{commitCenter:{x:f,y:u},cancelCenter:{x:f+d,y:u},buttonRadius:l}}function eo(t,e,s,i){const{commitCenter:a,cancelCenter:o,buttonRadius:r}=re(e,s,i);t.save(),t.lineCap="round",t.lineJoin="round",t.fillStyle="#ffffff",t.strokeStyle="#22c55e",t.lineWidth=1.5/i,t.beginPath(),t.arc(a.x,a.y,r,0,Math.PI*2),t.fill(),t.stroke(),t.strokeStyle="#16a34a",t.lineWidth=1.5/i,t.beginPath();const n=r*.4;t.moveTo(a.x-n,a.y+n*.1),t.lineTo(a.x-n*.15,a.y+n*.65),t.lineTo(a.x+n,a.y-n*.55),t.stroke(),t.fillStyle="#ffffff",t.strokeStyle="#ef4444",t.lineWidth=1.5/i,t.beginPath(),t.arc(o.x,o.y,r,0,Math.PI*2),t.fill(),t.stroke(),t.strokeStyle="#dc2626",t.lineWidth=1.5/i,t.beginPath();const c=r*.32;t.moveTo(o.x-c,o.y-c),t.lineTo(o.x+c,o.y+c),t.moveTo(o.x+c,o.y-c),t.lineTo(o.x-c,o.y+c),t.stroke(),t.restore()}function so(t,e,s,i){if(Zs(t,e,s,i))return"grab";const a=qs(t,e,s,i);return a?{nw:"nwse-resize",ne:"nesw-resize",se:"nwse-resize",sw:"nesw-resize",n:"ns-resize",s:"ns-resize",e:"ew-resize",w:"ew-resize"}[a]:Ks(t,e)?"move":"crosshair"}class rt{constructor(e,s,i,a,o){this._perspectiveCorners={nw:{x:0,y:0},ne:{x:0,y:0},se:{x:0,y:0},sw:{x:0,y:0}},this._perspectiveActive=!1,this._interaction={type:"idle"},this._handleConfig=Ms,this._sourceImageData=e,this._sourceRect=s,this._previewCanvas=i,this._zoom=a,this._pan=o,this._sourceCanvas=document.createElement("canvas"),this._sourceCanvas.width=e.width,this._sourceCanvas.height=e.height,this._sourceCanvas.getContext("2d").putImageData(e,0,0),this._state={x:s.x,y:s.y,width:s.w,height:s.h,rotation:0,skewX:0,skewY:0,scaleX:1,scaleY:1},this._initialState={...this._state},this.renderPreview()}get x(){return this._state.x}set x(e){this._state.x=e,this._onChange()}get y(){return this._state.y}set y(e){this._state.y=e,this._onChange()}get width(){return Math.abs(this._state.width*this._state.scaleX)}set width(e){e<=0||(this._state.scaleX=(this._state.scaleX<0?-1:1)*e/this._state.width,this._onChange())}get height(){return Math.abs(this._state.height*this._state.scaleY)}set height(e){e<=0||(this._state.scaleY=(this._state.scaleY<0?-1:1)*e/this._state.height,this._onChange())}get rotation(){return this._state.rotation*180/Math.PI}set rotation(e){this._state.rotation=e*Math.PI/180,this._onChange()}get skewX(){return this._state.skewX}set skewX(e){this._state.skewX=Math.max(-89,Math.min(89,e)),this._onChange()}get skewY(){return this._state.skewY}set skewY(e){this._state.skewY=Math.max(-89,Math.min(89,e)),this._onChange()}get flipH(){return this._state.scaleX<0}set flipH(e){const s=e,i=this._state.scaleX<0;s!==i&&(this._state.scaleX=-this._state.scaleX,this._onChange())}get flipV(){return this._state.scaleY<0}set flipV(e){const s=e,i=this._state.scaleY<0;s!==i&&(this._state.scaleY=-this._state.scaleY,this._onChange())}get perspectiveActive(){return this._perspectiveActive}setTouchMode(e){this._handleConfig=e?Va:Ms,this.renderPreview()}onPointerDown(e,s){const i=re(this._state,this._handleConfig,this._zoom);if(Math.hypot(e.x-i.commitCenter.x,e.y-i.commitCenter.y)<=i.buttonRadius||Math.hypot(e.x-i.cancelCenter.x,e.y-i.cancelCenter.y)<=i.buttonRadius)return!0;if(Zs(e,this._state,this._handleConfig,this._zoom)){const n=Wt(this._state),c=Math.atan2(e.y-n.y,e.x-n.x);return this._interaction={type:"rotating",startAngle:c,startRotation:this._state.rotation},!0}const r=qs(e,this._state,this._handleConfig,this._zoom);return r?(s.ctrl&&(r==="nw"||r==="ne"||r==="se"||r==="sw")?(this._perspectiveActive=!0,this._interaction={type:"perspective",corner:r,startPoint:e}):s.ctrl&&(r==="n"||r==="e"||r==="s"||r==="w")?this._interaction={type:"skewing",edge:r,startPoint:e,startSkewX:this._state.skewX,startSkewY:this._state.skewY}:this._interaction={type:"resizing",handle:r,origin:{rect:{x:this._state.x,y:this._state.y,w:this._state.width,h:this._state.height},point:e}},!0):Ks(e,this._state)?(this._interaction={type:"moving",startPoint:e,startX:this._state.x,startY:this._state.y},!0):(this._interaction={type:"outside-pending",startPoint:e},!0)}onPointerMove(e,s){switch(this._interaction.type){case"moving":this._handleMove(e,s);break;case"resizing":this._handleResize(e,s);break;case"rotating":this._handleRotate(e,s);break;case"skewing":this._handleSkew(e);break;case"perspective":this._handlePerspective(e);break;case"outside-pending":{const i=e.x-this._interaction.startPoint.x,a=e.y-this._interaction.startPoint.y;if(Math.sqrt(i*i+a*a)*this._zoom>Za){const r=Wt(this._state),n=Math.atan2(this._interaction.startPoint.y-r.y,this._interaction.startPoint.x-r.x);this._interaction={type:"rotating",startAngle:n,startRotation:this._state.rotation},this._handleRotate(e,s)}break}}}onPointerUp(e){const s=re(this._state,this._handleConfig,this._zoom);if(Math.hypot(e.x-s.commitCenter.x,e.y-s.commitCenter.y)<=s.buttonRadius)return this._interaction={type:"idle"},"commit-button";if(Math.hypot(e.x-s.cancelCenter.x,e.y-s.cancelCenter.y)<=s.buttonRadius)return this._interaction={type:"idle"},"cancel-button";const o=this._interaction.type==="outside-pending"?"commit":null;return this._interaction={type:"idle"},o}_handleMove(e,s){const i=this._interaction;if(i.type!=="moving")return;let a=e.x-i.startPoint.x,o=e.y-i.startPoint.y;s.shift&&(Math.abs(a)>Math.abs(o)?o=0:a=0),this._state.x=i.startX+a,this._state.y=i.startY+o,this._onChange()}_handleResize(e,s){const i=this._interaction;if(i.type!=="resizing")return;const{handle:a,origin:o}=i,{rect:r,point:n}=o,c=Re(e,this._state),h=Re(n,this._state),l=c.x-h.x,d=c.y-h.y;let f=r.x,u=r.y,p=r.w,_=r.h;if(a.includes("e")&&(p=r.w+l),a.includes("w")&&(f=r.x+l,p=r.w-l),a.includes("s")&&(_=r.h+d),a.includes("n")&&(u=r.y+d,_=r.h-d),s.shift&&(a==="nw"||a==="ne"||a==="se"||a==="sw")){const v=r.w/r.h;Math.abs(p/_)>v?_=p/v:p=_*v}const m=qa/this._zoom;Math.abs(p)<m&&(p=p<0?-m:m),Math.abs(_)<m&&(_=_<0?-m:m),this._state.x=f,this._state.y=u,this._state.width=Math.abs(p),this._state.height=Math.abs(_),p<0&&(this._state.scaleX=-Math.abs(this._state.scaleX)),_<0&&(this._state.scaleY=-Math.abs(this._state.scaleY)),this._onChange()}_handleRotate(e,s){const i=this._interaction;if(i.type!=="rotating")return;const a=Wt(this._state),o=Math.atan2(e.y-a.y,e.x-a.x);let r=i.startRotation+(o-i.startAngle);s.shift&&(r=Ga(r,Math.PI/12)),this._state.rotation=r,this._onChange()}_handleSkew(e){const s=this._interaction;if(s.type!=="skewing")return;const i=e.x-s.startPoint.x,a=e.y-s.startPoint.y;if(s.edge==="n"||s.edge==="s"){const o=s.edge==="n"?-1:1;this._state.skewX=Math.max(-89,Math.min(89,s.startSkewX+o*i*.5))}else{const o=s.edge==="w"?-1:1;this._state.skewY=Math.max(-89,Math.min(89,s.startSkewY+o*a*.5))}this._onChange()}_handlePerspective(e){const s=this._interaction;if(s.type!=="perspective")return;const i=e.x-s.startPoint.x,a=e.y-s.startPoint.y;this._perspectiveCorners[s.corner]={x:i,y:a},this._onChange()}renderPreview(){const e=this._previewCanvas.getContext("2d"),s=this._previewCanvas.width,i=this._previewCanvas.height;e.clearRect(0,0,s,i),e.save(),e.translate(this._pan.x,this._pan.y),e.scale(this._zoom,this._zoom);const a=this._perspectiveActive?ee(this._state,this._perspectiveCorners):[A({x:0,y:0},this._state),A({x:this._state.width,y:0},this._state),A({x:this._state.width,y:this._state.height},this._state),A({x:0,y:this._state.height},this._state)];e.save(),e.lineWidth=1/this._zoom,e.setLineDash([6/this._zoom,6/this._zoom]),e.strokeStyle="#ffffff",e.lineDashOffset=0,e.beginPath(),e.moveTo(a[0].x,a[0].y);for(let o=1;o<4;o++)e.lineTo(a[o].x,a[o].y);e.closePath(),e.stroke(),e.strokeStyle="#3b82f6",e.lineDashOffset=4/this._zoom,e.beginPath(),e.moveTo(a[0].x,a[0].y);for(let o=1;o<4;o++)e.lineTo(a[o].x,a[o].y);e.closePath(),e.stroke(),e.restore(),Qa(e,this._state,this._handleConfig,this._zoom),to(e,this._state,this._handleConfig,this._zoom),eo(e,this._state,this._handleConfig,this._zoom),e.restore()}renderTransformed(e){if(this._perspectiveActive){const s=[{x:0,y:0},{x:this._sourceCanvas.width,y:0},{x:this._sourceCanvas.width,y:this._sourceCanvas.height},{x:0,y:this._sourceCanvas.height}],i=ee(this._state,this._perspectiveCorners),a=8,o=i.map(u=>u.x),r=i.map(u=>u.y),n=Math.floor(Math.min(...o)),c=Math.floor(Math.min(...r)),h=Math.ceil(Math.max(...o)),l=Math.ceil(Math.max(...r)),d=h-n,f=l-c;if(d>0&&f>0){const u=document.createElement("canvas");u.width=d,u.height=f;const p=u.getContext("2d");p.translate(-n,-c),Ps(p,this._sourceCanvas,s,i,a),e.drawImage(u,n,c)}}else{const s=de(this._state);e.save(),e.transform(s.a,s.b,s.c,s.d,s.e,s.f),e.drawImage(this._sourceCanvas,0,0,this._state.width,this._state.height),e.restore()}}snapshot(){const e=this._getSnapshotBounds(),s=document.createElement("canvas");s.width=e.w,s.height=e.h;const i=s.getContext("2d");return i.save(),i.translate(-e.x,-e.y),this.renderTransformed(i),i.restore(),{canvas:s,...e}}commit(e){const s=e.getContext("2d");if(this._perspectiveActive){const i=[{x:0,y:0},{x:this._sourceCanvas.width,y:0},{x:this._sourceCanvas.width,y:this._sourceCanvas.height},{x:0,y:this._sourceCanvas.height}],a=ee(this._state,this._perspectiveCorners);Ps(s,this._sourceCanvas,i,a,32)}else{const i=de(this._state);s.save(),s.setTransform(i.a,i.b,i.c,i.d,i.e,i.f),s.drawImage(this._sourceCanvas,0,0,this._state.width,this._state.height),s.restore()}}cancel(){return this._sourceImageData}hasChanged(){const e=this._state,s=this._initialState;return e.x!==s.x||e.y!==s.y||e.width!==s.width||e.height!==s.height||e.rotation!==s.rotation||e.skewX!==s.skewX||e.skewY!==s.skewY||e.scaleX!==s.scaleX||e.scaleY!==s.scaleY||this._perspectiveActive}getState(){return this._state}getSourceRect(){return this._sourceRect}getBounds(){return this._getSnapshotBounds()}updateViewport(e,s){this._zoom=e,this._pan=s,this.renderPreview()}getCursor(e){const s=re(this._state,this._handleConfig,this._zoom);return Math.hypot(e.x-s.commitCenter.x,e.y-s.commitCenter.y)<=s.buttonRadius||Math.hypot(e.x-s.cancelCenter.x,e.y-s.cancelCenter.y)<=s.buttonRadius?"pointer":so(e,this._state,this._handleConfig,this._zoom)}_onChange(){this.renderPreview()}_getSnapshotBounds(){const e=this._perspectiveActive?ee(this._state,this._perspectiveCorners):[A({x:0,y:0},this._state),A({x:this._state.width,y:0},this._state),A({x:this._state.width,y:this._state.height},this._state),A({x:0,y:this._state.height},this._state)],s=e.map(c=>c.x),i=e.map(c=>c.y),a=Math.floor(Math.min(...s)),o=Math.floor(Math.min(...i)),r=Math.max(1,Math.ceil(Math.max(...s))-a),n=Math.max(1,Math.ceil(Math.max(...i))-o);return{x:a,y:o,w:r,h:n}}dispose(){}}var io=Object.getOwnPropertyDescriptor,ao=(t,e,s,i)=>{for(var a=i>1?void 0:i?io(e,s):e,o=t.length-1,r;o>=0;o--)(r=t[o])&&(a=r(a)||a);return a};let Ee=class extends F{constructor(){super(...arguments),this._dialog=null,this._resolve=null,this._imgW=0,this._imgH=0,this._canvasW=0,this._canvasH=0}show(t,e,s,i){return new Promise(a=>{this._resolve=a,this._imgW=t,this._imgH=e,this._canvasW=s,this._canvasH=i,this.requestUpdate(),this.updateComplete.then(()=>{this._dialog=this.renderRoot.querySelector("dialog"),this._dialog?.showModal()})})}_onScale(){this._dialog?.close(),this._resolve?.(!0),this._resolve=null}_onKeep(){this._dialog?.close(),this._resolve?.(!1),this._resolve=null}render(){return g`
      <dialog @cancel=${t=>{t.preventDefault(),this._onKeep()}}>
        <p>
          This image (${this._imgW}&times;${this._imgH}) is larger than the canvas
          (${this._canvasW}&times;${this._canvasH}). Would you like to scale it to fit?
        </p>
        <div class="buttons">
          <button @click=${this._onKeep}>Keep original size</button>
          <button class="primary" @click=${this._onScale}>Scale to fit</button>
        </div>
      </dialog>
    `}};Ee.styles=bt`
    dialog {
      background: #2a2a2a;
      color: #e0e0e0;
      border: 1px solid #555;
      border-radius: 8px;
      padding: 24px;
      max-width: 400px;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 14px;
    }
    dialog::backdrop {
      background: rgba(0, 0, 0, 0.5);
    }
    p {
      margin: 0 0 16px;
      line-height: 1.5;
    }
    .buttons {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
    }
    button {
      padding: 8px 16px;
      border-radius: 4px;
      border: 1px solid #555;
      background: #3a3a3a;
      color: #e0e0e0;
      cursor: pointer;
      font-size: 13px;
    }
    button:hover {
      background: #4a4a4a;
    }
    button.primary {
      background: #4a90d9;
      border-color: #4a90d9;
    }
    button.primary:hover {
      background: #5aa0e9;
    }
  `;Ee=ao([yt("resize-dialog")],Ee);var oo=Object.defineProperty,ro=Object.getOwnPropertyDescriptor,Zt=(t,e,s,i)=>{for(var a=i>1?void 0:i?ro(e,s):e,o=t.length-1,r;o>=0;o--)(r=t[o])&&(a=(i?r(e,s,a):r(a))||a);return i&&a&&oo(e,s,a),a};function $s(t){return t.before.width===1&&t.before.height===1&&!$e(t.before,t.after)}function no(t){return(t??"").replace(/[\\/:*?"<>|\u0000-\u001f\u007f]+/g,"-").replace(/\s+/g," ").replace(/^[.\s]+|[.\s]+$/g,"")||"drawing"}let R=class extends F{constructor(){super(...arguments),this._ctx=new gt(this,{context:Et,subscribe:!0}),this._checkerboardPattern=null,this._resizeObserver=null,this._lastLayers=null,this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._panX=0,this._panY=0,this._panning=!1,this._panStartX=0,this._panStartY=0,this._panStartOffsetX=0,this._panStartOffsetY=0,this._panPointerId=-1,this._moveTempCanvas=null,this._moveStartPoint=null,this._zoom=1,this._pointers=new Map,this._pinching=!1,this._lastPinchDist=0,this._lastPinchMidX=0,this._lastPinchMidY=0,this._transformManager=null,this._transformContentMode="lifted",this._clipboard=null,this._clipboardOrigin=null,this._clipboardRotation=0,this._clipboardBlobSize=null,this._engine=new Us,this._tintPreviewCanvas=null,this._strokeTintCanvas=null,this._samplingDirty=!0,this._compositeScheduler=Ne(()=>this.composite()),this._canvasRect=null,this._strokeTintNeedsClear=!0,this._samplingBuffer=null,this._altSampling=!1,this._lastPointerScreenX=0,this._lastPointerScreenY=0,this._pointerOnCanvas=!1,this._floatIsExternalImage=!1,this._selectionDrawing=!1,this._cropRectValue=null,this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._cropActionsVisible=!1,this._textEditing=!1,this._textPosition={x:0,y:0},this._textSelecting=!1,this._textSelectAnchor=0,this._textAreaEl=null,this._textCursorVisible=!1,this._textCursorInterval=0,this._history=[],this._historyIndex=-1,this._maxHistory=50,this._historyTrimmed=0,this._beforeDrawCanvas=null,this._beforeDrawBuffer=null,this._invalidateCanvasRect=()=>{this._canvasRect=null},this._onWheel=t=>{if(t.ctrlKey||t.metaKey){if(t.preventDefault(),t.deltaY===0)return;const e=this._getCanvasRect(),s=t.clientX-e.left,i=t.clientY-e.top,a=(s-this._panX)/this._zoom,o=(i-this._panY)/this._zoom,r=Math.max(-5,Math.min(5,-t.deltaY*.01)),n=Math.min(R.MAX_ZOOM,Math.max(R.MIN_ZOOM,this._zoom*(1+r)));if(n===this._zoom)return;this._panX=s-a*n,this._panY=i-o*n,this._zoom=n,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange();return}t.preventDefault(),this._panX-=t.deltaX,this._panY-=t.deltaY,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange()},this._laidOut=!1,this._onWindowBlur=()=>{this._altSampling=!1,this._clearEyedropperPreview()},this._onPointerEnter=t=>{this._pointerOnCanvas=!0;const e=this._getCanvasRect();this._lastPointerScreenX=t.clientX-e.left,this._lastPointerScreenY=t.clientY-e.top,this._renderPreview()},this._onApplyCropClick=()=>{this.commitCrop()},this._onCancelCropClick=()=>{this.cancelCrop()},this._onDragOver=t=>{t.preventDefault()},this._onDragEnter=t=>{t.preventDefault(),this.classList.add("drop-target")},this._onDragLeave=t=>{t.relatedTarget&&this.contains(t.relatedTarget)||this.classList.remove("drop-target")},this._onDrop=async t=>{if(t.preventDefault(),this.classList.remove("drop-target"),!!t.dataTransfer?.files.length)for(const e of Array.from(t.dataTransfer.files)){if(!e.type.startsWith("image/"))continue;const s=URL.createObjectURL(e),i=e.name.replace(/\.[^.]+$/,"")||"Dropped Image";try{const a=await new Promise((o,r)=>{const n=new Image;n.onload=()=>o(n),n.onerror=()=>r(new Error("Image load failed")),n.src=s});URL.revokeObjectURL(s),await this._handleExternalImage(a,i)}catch{URL.revokeObjectURL(s)}return}}}get ctx(){return this._ctx.value}get _cropRect(){return this._cropRectValue}set _cropRect(t){this._cropRectValue=t,this._updateCropActions()}_updateCropActions(){const t=this._cropRectValue;this._cropActionsVisible=t!==null&&!this._cropDragging&&this._cropHandle===null&&Math.abs(t.w)>=1&&Math.abs(t.h)>=1}get _docWidth(){return this._ctx.value?.state.documentWidth??800}get _docHeight(){return this._ctx.value?.state.documentHeight??600}getWidth(){return this._docWidth}getHeight(){return this._docHeight}invalidateSamplingBuffer(){this._samplingDirty=!0}scheduleComposite(){this._compositeScheduler.schedule()}isTransformActive(){return this._transformManager!==null}hasPendingText(){return this._textEditing&&!!this._textAreaEl?.value}_dispatchPendingTextChange(){this.dispatchEvent(new CustomEvent("pending-text-change",{bubbles:!0,composed:!0,detail:{pending:this.hasPendingText()}}))}enterTransformMode(){if(this._transformManager)return;const t=this._ctx.value?.state;if(!t)return;const e=t.layers.find(r=>r.id===t.activeLayerId);if(!e)return;const s=e.canvas.getContext("2d"),i=s.getImageData(0,0,e.canvas.width,e.canvas.height),a=ks(i);if(!a)return;this._captureBeforeDraw();const o=s.getImageData(a.x,a.y,a.w,a.h);s.clearRect(0,0,e.canvas.width,e.canvas.height),this._transformContentMode="lifted",this._transformManager=new rt(o,a,this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}getTransformValues(){if(!this._transformManager)return null;const t=this._transformManager;return{x:t.x,y:t.y,width:t.width,height:t.height,rotation:t.rotation,skewX:t.skewX,skewY:t.skewY,flipH:t.flipH,flipV:t.flipV}}setTransformValue(t,e){if(!this._transformManager)return;const s=this._transformManager;switch(t){case"x":s.x=e;break;case"y":s.y=e;break;case"width":s.width=e;break;case"height":s.height=e;break;case"rotation":s.rotation=e;break;case"skewX":s.skewX=e;break;case"skewY":s.skewY=e;break;case"flipH":s.flipH=!s.flipH;break;case"flipV":s.flipV=!s.flipV;break}this.composite(),this.requestUpdate(),this._dispatchTransformChange()}commitTransform(){if(!this._transformManager)return;const t=this._ctx.value?.state;if(!t)return;const e=t.layers.find(o=>o.id===t.activeLayerId);if(!e)return;const s=e.canvas.getContext("2d"),i=this._transformContentMode==="inserted";let a=null;if(i){const o=this._transformManager.getBounds(),r=Math.max(0,o.x),n=Math.max(0,o.y),c=Math.min(this._docWidth,o.x+o.w),h=Math.min(this._docHeight,o.y+o.h),l=c-r,d=h-n;l>0&&d>0&&(a={x:r,y:n,w:l,h:d,before:s.getImageData(r,n,l,d)})}if(this._transformManager.commit(e.canvas),i&&a){const o=s.getImageData(a.x,a.y,a.w,a.h),r=$e(a.before,o);r&&this._pushHistoryEntry({type:"patch",layerId:e.id,x:a.x+r.x,y:a.y+r.y,before:Gt(a.before,r),after:Gt(o,r)})}else if(this._transformManager.hasChanged()&&this._beforeDrawCanvas){const o=this._readChangedPatch(s,void 0,!0);o&&this._pushHistoryEntry({type:"patch",layerId:e.id,...o})}this._beforeDrawCanvas=null,this._transformContentMode="lifted",this._floatIsExternalImage=!1,this._transformManager.dispose(),this._transformManager=null,this.previewCanvas.getContext("2d").clearRect(0,0,this.previewCanvas.width,this.previewCanvas.height),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}cancelTransform(){if(!this._transformManager)return;const t=this._ctx.value?.state;if(!t)return;const e=t.layers.find(i=>i.id===t.activeLayerId);if(!e)return;if(this._floatIsExternalImage){this.cancelExternalFloat();return}const s=e.canvas.getContext("2d");if(this._transformContentMode==="inserted")this._transformManager.cancel();else if(this._beforeDrawCanvas)this._restoreBeforeDraw(s);else{const i=this._transformManager.cancel(),a=this._transformManager.getSourceRect();s.putImageData(i,a.x,a.y)}this._transformManager.dispose(),this._transformManager=null,this._beforeDrawCanvas=null,this._transformContentMode="lifted",this.previewCanvas.getContext("2d").clearRect(0,0,this.previewCanvas.width,this.previewCanvas.height),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}get _vw(){return this.mainCanvas?.width??800}get _vh(){return this.mainCanvas?.height??600}get _brushDescriptor(){const t=this.ctx.state;if(t.brush)return t.brush;const e=ie();return{...e,size:t.brushSize??e.size,tip:{...e.tip},ink:{...e.ink}}}_getActiveLayerCtx(){const t=this._ctx.value?.state;return t?t.layers.find(s=>s.id===t.activeLayerId)?.canvas.getContext("2d")??null:null}composite(){if(!this.mainCanvas)return;this._compositeScheduler.cancel();const t=this.mainCanvas.getContext("2d"),e=this._vw,s=this._vh;t.fillStyle="#3a3a3a",t.fillRect(0,0,e,s),t.save(),t.translate(this._panX,this._panY),t.scale(this._zoom,this._zoom),t.save(),t.beginPath(),t.rect(0,0,this._docWidth,this._docHeight),t.clip();const i=this._getCheckerboardPattern(t);t.fillStyle=i,t.fillRect(0,0,this._docWidth,this._docHeight),t.restore();const a=this._ctx.value?.state.layers??[],o=this._ctx.value?.state.activeLayerId??null,r=a.some(n=>n.visible&&n.blendMode!=="normal");for(const n of a){if(!n.visible)continue;t.globalAlpha=n.opacity,r&&(t.globalCompositeOperation=Yt(n.blendMode));const c=this._drawing&&n.id===o?this._engine.getStrokePreview():null,h=c?.bounds??null;if(c&&h)if(!c.eraser&&n.opacity>=1&&n.blendMode==="normal"){t.drawImage(n.canvas,0,0);const d=c.color===null?c.canvas:this._tintStrokeRegion(c.canvas,h,c.color);t.globalAlpha=c.opacity,t.drawImage(d,h.x,h.y,h.w,h.h,h.x,h.y,h.w,h.h),t.globalAlpha=n.opacity}else{(!this._tintPreviewCanvas||this._tintPreviewCanvas.width!==this._docWidth||this._tintPreviewCanvas.height!==this._docHeight)&&(this._tintPreviewCanvas=document.createElement("canvas"),this._tintPreviewCanvas.width=this._docWidth,this._tintPreviewCanvas.height=this._docHeight);const d=this._tintPreviewCanvas.getContext("2d");if(d.globalCompositeOperation="source-over",d.clearRect(0,0,this._docWidth,this._docHeight),d.drawImage(n.canvas,0,0),d.globalAlpha=c.opacity,c.eraser)d.globalCompositeOperation="destination-out",d.drawImage(c.canvas,h.x,h.y,h.w,h.h,h.x,h.y,h.w,h.h);else if(c.color===null)d.drawImage(c.canvas,h.x,h.y,h.w,h.h,h.x,h.y,h.w,h.h);else{const f=this._tintStrokeRegion(c.canvas,h,c.color);d.drawImage(f,h.x,h.y,h.w,h.h,h.x,h.y,h.w,h.h)}d.globalAlpha=1,d.globalCompositeOperation="source-over",t.drawImage(this._tintPreviewCanvas,0,0)}else t.drawImage(n.canvas,0,0);this._transformManager&&n.id===o&&this._transformManager.renderTransformed(t),r&&(t.globalCompositeOperation="source-over"),t.globalAlpha=1}t.strokeStyle="rgba(0,0,0,0.3)",t.lineWidth=1,t.strokeRect(-.5,-.5,this._docWidth+1,this._docHeight+1),t.restore(),this.dispatchEvent(new CustomEvent("composited",{bubbles:!0,composed:!0,detail:null})),this.invalidateSamplingBuffer()}_tintStrokeRegion(t,e,s){const i=this._docWidth,a=this._docHeight;(!this._strokeTintCanvas||this._strokeTintCanvas.width!==i||this._strokeTintCanvas.height!==a)&&(this._strokeTintCanvas=document.createElement("canvas"),this._strokeTintCanvas.width=i,this._strokeTintCanvas.height=a,this._strokeTintNeedsClear=!1);const o=this._strokeTintCanvas.getContext("2d");return this._strokeTintNeedsClear&&(o.clearRect(0,0,i,a),this._strokeTintNeedsClear=!1),o.save(),o.beginPath(),o.rect(e.x,e.y,e.w,e.h),o.clip(),o.globalCompositeOperation="source-over",o.clearRect(e.x,e.y,e.w,e.h),o.drawImage(t,e.x,e.y,e.w,e.h,e.x,e.y,e.w,e.h),Xe(o,s,i,a),o.restore(),this._strokeTintCanvas}_getCheckerboardPattern(t){if(!this._checkerboardPattern){const e=document.createElement("canvas");e.width=20,e.height=20;const s=e.getContext("2d");s.fillStyle="#ffffff",s.fillRect(0,0,20,20),s.fillStyle="#e0e0e0",s.fillRect(10,0,10,10),s.fillRect(0,10,10,10),this._checkerboardPattern=t.createPattern(e,"repeat")}return this._checkerboardPattern}willUpdate(){const t=this._ctx.value?.state.layers??null;if(t&&t!==this._lastLayers&&(this._lastLayers=t,this.mainCanvas&&this.composite()),this.mainCanvas&&this._ctx.value){const e=this._ctx.value.state.activeTool;e==="hand"?this.mainCanvas.style.cursor=this._panning?"grabbing":"grab":e==="move"?this.mainCanvas.style.cursor="move":e==="text"?this.mainCanvas.style.cursor="text":this.mainCanvas.style.cursor="crosshair",this._textEditing&&this._renderTextPreview()}this._renderPreview()}firstUpdated(){const t=this.getBoundingClientRect(),e=t.width>0?Math.floor(t.width):800,s=t.height>0?Math.floor(t.height):600;this._laidOut=t.width>0&&t.height>0,this.mainCanvas.width=e,this.mainCanvas.height=s,this.previewCanvas.width=e,this.previewCanvas.height=s,this._zoom=Math.max(R.MIN_ZOOM,Math.min(this._zoom,this._fitZoom())),this._panX=Math.round((e-this._docWidth*this._zoom)/2),this._panY=Math.round((s-this._docHeight*this._zoom)/2),this._resizeObserver=new ResizeObserver(()=>{this._invalidateCanvasRect(),this._resizeToFit()}),this._resizeObserver.observe(this);const i=this._getActiveLayerCtx();i&&(i.fillStyle="#ffffff",i.fillRect(0,0,this._docWidth,this._docHeight)),this.composite(),this._dispatchViewportChange(),this._textAreaEl&&this.shadowRoot.appendChild(this._textAreaEl)}centerDocument(){this.mainCanvas&&(this._panX=Math.round((this._vw-this._docWidth*this._zoom)/2),this._panY=Math.round((this._vh-this._docHeight*this._zoom)/2),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange())}_resizeToFit(){const t=this.getBoundingClientRect();if(t.width<=0||t.height<=0)return;const e=Math.floor(t.width),s=Math.floor(t.height);this._laidOut=!0;const i=this.mainCanvas.width,a=this.mainCanvas.height;if(i===e&&a===s)return;this.mainCanvas.width=e,this.mainCanvas.height=s,this.previewCanvas.width=e,this.previewCanvas.height=s;const o=(i/2-this._panX)/this._zoom,r=(a/2-this._panY)/this._zoom;this._panX=e/2-o*this._zoom,this._panY=s/2-r*this._zoom,this._checkerboardPattern=null,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange()}getHistory(){return[...this._history]}getHistoryIndex(){return this._historyIndex}getHistoryTrimmedCount(){return this._historyTrimmed}setHistory(t,e){this._history=t,this._historyIndex=Math.max(-1,Math.min(e,t.length-1)),this._notifyHistory()}_captureBeforeDraw(){const t=this._getActiveLayerCtx();if(!t)return;const{width:e,height:s}=t.canvas;let i=this._beforeDrawBuffer;(!i||i.width!==e||i.height!==s)&&(i=document.createElement("canvas"),i.width=e,i.height=s,this._beforeDrawBuffer=i);const a=i.getContext("2d");a.clearRect(0,0,e,s),a.drawImage(t.canvas,0,0),this._beforeDrawCanvas=i}_restoreBeforeDraw(t){const e=this._beforeDrawCanvas;e&&(t.save(),t.setTransform(1,0,0,1,0,0),t.globalAlpha=1,t.globalCompositeOperation="source-over",t.clearRect(0,0,t.canvas.width,t.canvas.height),t.drawImage(e,0,0),t.restore())}_pushDrawHistory(t=!1,e){const s=this._ctx.value?.state,i=this._getActiveLayerCtx();if(!i||!s||!this._beforeDrawCanvas)return;const a=this._readChangedPatch(i,e,t);this._beforeDrawCanvas=null,a&&this._pushHistoryEntry({type:"patch",layerId:s.activeLayerId,...a})}_readChangedPatch(t,e,s){const i=this._beforeDrawCanvas,a=Math.min(i.width,t.canvas.width),o=Math.min(i.height,t.canvas.height),r=e??{x:0,y:0,w:a,h:o},n=Math.max(0,Math.floor(r.x)),c=Math.max(0,Math.floor(r.y)),h=Math.min(a,Math.ceil(r.x+r.w))-n,l=Math.min(o,Math.ceil(r.y+r.h))-c,d=i.getContext("2d");let f=null,u=null,p=null;return h>0&&l>0&&(u=d.getImageData(n,c,h,l),p=t.getImageData(n,c,h,l),f=$e(u,p)),f&&u&&p?{x:n+f.x,y:c+f.y,before:Gt(u,f),after:Gt(p,f)}:!s||a<=0||o<=0?null:{x:0,y:0,before:d.getImageData(0,0,1,1),after:t.getImageData(0,0,1,1)}}_commitStroke(t){if(!this._engine.commit(t))return;const e=this._engine.getDirtyBounds();if(!e)return{x:0,y:0,w:0,h:0};const s=2;return{x:e.x-s,y:e.y-s,w:e.w+s*2,h:e.h+s*2}}pushLayerOperation(t){this._pushHistoryEntry(t)}_pushHistoryEntry(t){this._history=this._history.slice(0,this._historyIndex+1),this._history.push(t),this._history.length>this._maxHistory?(this._history.shift(),this._historyTrimmed++):this._historyIndex++,this._notifyHistory()}_getEntryLayerId(t){switch(t.type){case"draw":case"patch":case"visibility":case"opacity":case"rename":case"blend-mode":case"transform":return t.layerId;case"add-layer":case"delete-layer":return t.layer.id;case"reorder":return null;case"crop":case"merge":return null}}_notifyHistory(){this.dispatchEvent(new CustomEvent("history-change",{bubbles:!0,composed:!0,detail:{canUndo:this._historyIndex>=0||this._transformManager!==null,canRedo:this._historyIndex<this._history.length-1}}))}_dispatchTransformChange(){this.dispatchEvent(new CustomEvent("transform-change",{bubbles:!0,composed:!0,detail:{active:this._transformManager!==null,values:this.getTransformValues()}}))}_transformValuesEqual(t,e){return t===e?!0:!t||!e?!1:t.x===e.x&&t.y===e.y&&t.width===e.width&&t.height===e.height&&t.rotation===e.rotation&&t.skewX===e.skewX&&t.skewY===e.skewY&&t.flipH===e.flipH&&t.flipV===e.flipV}undo(){if(this._textEditing&&this._commitText(),this._drawing){const e=this._getActiveLayerCtx(),s=e?this._commitStroke(e):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null;const i=this._beforeDrawCanvas!==null;if(this._pushDrawHistory(!1,s),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),!i)return}if(this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._transformManager){this.cancelTransform();return}if(this._historyIndex<0)return;const t=this._history[this._historyIndex];this._historyIndex--,this._applyUndo(t),this.composite(),this._notifyHistory()}redo(){if(this._textEditing&&this._commitText(),this._drawing){const e=this._getActiveLayerCtx(),s=e?this._commitStroke(e):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null;const i=this._beforeDrawCanvas!==null;if(this._pushDrawHistory(!1,s),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),!i)return}if(this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._historyIndex>=this._history.length-1)return;this._transformManager&&this.cancelTransform(),this._historyIndex++;const t=this._history[this._historyIndex];this._applyRedo(t),this.composite(),this._notifyHistory()}_applyUndo(t){const e=this._ctx.value?.state;if(e)switch(t.type){case"draw":case"transform":{const s=e.layers.find(i=>i.id===t.layerId);s&&s.canvas.getContext("2d").putImageData(t.before,0,0);break}case"patch":{const s=e.layers.find(i=>i.id===t.layerId);s&&!$s(t)&&s.canvas.getContext("2d").putImageData(t.before,t.x,t.y);break}case"add-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:t.layer.id}}));break}case"delete-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"restore-layer",snapshot:t.layer,index:t.index}}));break}case"reorder":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"reorder",fromIndex:t.toIndex,toIndex:t.fromIndex}}));break}case"visibility":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.visible=t.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"opacity":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.opacity=t.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"rename":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.name=t.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"blend-mode":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.blendMode=t.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"crop":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"crop-restore",layers:t.beforeLayers,width:t.beforeWidth,height:t.beforeHeight}}));break}case"merge":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"stack-replace",layers:t.beforeLayers,activeLayerId:t.previousActiveLayerId}}));break}}}_applyRedo(t){const e=this._ctx.value?.state;if(e)switch(t.type){case"draw":case"transform":{const s=e.layers.find(i=>i.id===t.layerId);s&&s.canvas.getContext("2d").putImageData(t.after,0,0);break}case"patch":{const s=e.layers.find(i=>i.id===t.layerId);s&&!$s(t)&&s.canvas.getContext("2d").putImageData(t.after,t.x,t.y);break}case"add-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"restore-layer",snapshot:t.layer,index:t.index}}));break}case"delete-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:t.layer.id}}));break}case"reorder":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"reorder",fromIndex:t.fromIndex,toIndex:t.toIndex}}));break}case"visibility":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.visible=t.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"opacity":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.opacity=t.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"rename":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.name=t.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"blend-mode":{const s=e.layers.find(i=>i.id===t.layerId);s&&(s.blendMode=t.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"crop":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"crop-restore",layers:t.afterLayers,width:t.afterWidth,height:t.afterHeight}}));break}case"merge":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"stack-replace",layers:t.afterLayers,activeLayerId:t.afterActiveLayerId}}));break}}}clearCanvas(){if(this._drawing){const e=this._getActiveLayerCtx(),s=e?this._commitStroke(e):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!0,s)}this.clearSelection(),this._captureBeforeDraw();const t=this._getActiveLayerCtx();t&&t.clearRect(0,0,this._docWidth,this._docHeight),this._pushDrawHistory(!0),this.composite()}renderFlattened(t="#ffffff"){const e=document.createElement("canvas");e.width=this._docWidth,e.height=this._docHeight;const s=e.getContext("2d");t&&(s.fillStyle=t,s.fillRect(0,0,this._docWidth,this._docHeight));const i=this._ctx.value?.state,a=i?.layers??[],o=i?.activeLayerId??null;for(const r of a)r.visible&&(s.globalAlpha=r.opacity,s.globalCompositeOperation=Yt(r.blendMode),s.drawImage(r.canvas,0,0),this._transformManager&&r.id===o&&this._transformManager.renderTransformed(s),s.globalCompositeOperation="source-over",s.globalAlpha=1);return e}saveCanvas(){const t=this.renderFlattened("#ffffff"),e=document.createElement("a");e.download=`${no(this._ctx.value?.currentProject?.name)}.png`,e.href=t.toDataURL("image/png"),e.click()}_getCanvasRect(){return this._canvasRect||(this._canvasRect=this.mainCanvas.getBoundingClientRect()),this._canvasRect}_getDocPoint(t){const e=this._getCanvasRect();return{x:(t.clientX-e.left-this._panX)/this._zoom,y:(t.clientY-e.top-this._panY)/this._zoom}}_clientToDoc(t,e){const s=this._getCanvasRect();return{x:(t-s.left-this._panX)/this._zoom,y:(e-s.top-this._panY)/this._zoom}}_startPan(t){this._panning=!0,this._panStartX=t.clientX,this._panStartY=t.clientY,this._panStartOffsetX=this._panX,this._panStartOffsetY=this._panY,this._panPointerId=t.pointerId,this.mainCanvas.setPointerCapture(t.pointerId),this.mainCanvas.style.cursor="grabbing"}_updatePan(t){this._panning&&(this._panX=this._panStartOffsetX+(t.clientX-this._panStartX),this._panY=this._panStartOffsetY+(t.clientY-this._panStartY),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview())}_endPan(){if(!this._panning)return;const t=this._panPointerId;if(this._panning=!1,this._panPointerId=-1,t>=0&&this.mainCanvas)try{this.mainCanvas.releasePointerCapture(t)}catch{}if(this._ctx.value){const e=this._ctx.value.state.activeTool;e==="hand"?this.mainCanvas.style.cursor="grab":e==="move"?this.mainCanvas.style.cursor="move":this.mainCanvas.style.cursor="crosshair"}this._dispatchViewportChange()}_dispatchZoomChange(){this.dispatchEvent(new CustomEvent("zoom-change",{bubbles:!0,composed:!0,detail:{zoom:this._zoom}})),this._dispatchViewportChange()}_dispatchViewportChange(){this.dispatchEvent(new CustomEvent("viewport-change",{bubbles:!0,composed:!0}))}zoomIn(){this._zoomToCenter(this._zoom*R.ZOOM_STEP)}zoomOut(){this._zoomToCenter(this._zoom/R.ZOOM_STEP)}resetView(){this.mainCanvas&&this._setCenteredZoom(Math.min(1,this._fitZoom()))}zoomToFit(){this._setCenteredZoom(this._fitZoom())}_fitZoom(){return Math.min(this._vw/this._docWidth,this._vh/this._docHeight)*.9}_setCenteredZoom(t){this._zoom=Math.min(R.MAX_ZOOM,Math.max(R.MIN_ZOOM,t)),this._panX=Math.round((this._vw-this._docWidth*this._zoom)/2),this._panY=Math.round((this._vh-this._docHeight*this._zoom)/2),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange()}getZoom(){return this._zoom}getViewport(){return{zoom:this._zoom,panX:this._panX,panY:this._panY}}getViewportSize(){return this._laidOut?{width:this._vw,height:this._vh}:null}restoreViewport(t,e,s,i){if(i&&this._laidOut){if(!R._similarSize(i,{width:this._vw,height:this._vh})){this.resetView();return}e+=(this._vw-i.width)/2,s+=(this._vh-i.height)/2}this.setViewport(t,e,s),this._visibleDocumentFraction()<.5&&this.resetView()}static _similarSize(t,e){const s=(i,a)=>Math.abs(i-a)<=Math.max(i,a)*.2;return s(t.width,e.width)&&s(t.height,e.height)}_visibleDocumentFraction(){const t=this._docWidth*this._zoom,e=this._docHeight*this._zoom,s=Math.max(0,Math.min(this._vw,this._panX+t)-Math.max(0,this._panX)),i=Math.max(0,Math.min(this._vh,this._panY+e)-Math.max(0,this._panY)),a=Math.min(t,this._vw)*Math.min(e,this._vh);return a>0?s*i/a:0}setViewport(t,e,s){this._zoom=Math.min(R.MAX_ZOOM,Math.max(R.MIN_ZOOM,t)),this._panX=e,this._panY=s,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange()}_zoomToCenter(t){const e=Math.min(R.MAX_ZOOM,Math.max(R.MIN_ZOOM,t));if(e===this._zoom)return;const s=this._vw/2,i=this._vh/2,a=(s-this._panX)/this._zoom,o=(i-this._panY)/this._zoom;this._panX=s-a*e,this._panY=i-o*e,this._zoom=e,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange()}_ensureSamplingBuffer(){(!this._samplingBuffer||this._samplingBuffer.width!==this._docWidth||this._samplingBuffer.height!==this._docHeight)&&(this._samplingBuffer=document.createElement("canvas"),this._samplingBuffer.width=this._docWidth,this._samplingBuffer.height=this._docHeight,this._samplingDirty=!0);const t=this._samplingBuffer.getContext("2d",{willReadFrequently:!0});if(this._samplingDirty){t.clearRect(0,0,this._docWidth,this._docHeight),t.fillStyle="#ffffff",t.fillRect(0,0,this._docWidth,this._docHeight);const e=this._ctx.value?.state.layers??[],s=this._ctx.value?.state.activeLayerId??null;for(const i of e)i.visible&&(t.globalAlpha=i.opacity,t.globalCompositeOperation=Yt(i.blendMode),t.drawImage(i.canvas,0,0),t.globalCompositeOperation="source-over",this._transformManager&&i.id===s&&this._transformManager.renderTransformed(t));t.globalAlpha=1,this._samplingDirty=!1}return t}_sampleColor(t,e){const s=Math.round(t),i=Math.round(e);if(s<0||i<0||s>=this._docWidth||i>=this._docHeight)return null;if(this.ctx.state.eyedropperSampleAll){const r=this._ensureSamplingBuffer().getImageData(s,i,1,1).data;return`#${r[0].toString(16).padStart(2,"0")}${r[1].toString(16).padStart(2,"0")}${r[2].toString(16).padStart(2,"0")}`}else{const o=this._getActiveLayerCtx();if(!o)return null;const r=o.getImageData(s,i,1,1).data;if(r[3]===0)return null;if(r[3]<255){const n=r[3]/255,c=Math.round(r[0]*n+255*(1-n)),h=Math.round(r[1]*n+255*(1-n)),l=Math.round(r[2]*n+255*(1-n));return`#${c.toString(16).padStart(2,"0")}${h.toString(16).padStart(2,"0")}${l.toString(16).padStart(2,"0")}`}return`#${r[0].toString(16).padStart(2,"0")}${r[1].toString(16).padStart(2,"0")}${r[2].toString(16).padStart(2,"0")}`}}_renderEyedropperPreview(t){const e=this.previewCanvas.getContext("2d");e.clearRect(0,0,this._vw,this._vh);const s=this._getDocPoint(t),i=this._sampleColor(s.x,s.y),a=this.ctx.state.eyedropperSampleAll,o=a?this.mainCanvas:this._getActiveLayerCtx()?.canvas??this.mainCanvas,r=88,n=24,c=r+n+4,h=20,l=this._getCanvasRect();let d=t.clientX-l.left+h,f=t.clientY-l.top-h-c;if(d+r>this._vw&&(d=d-r-2*h),f<0&&(f=f+c+2*h),e.save(),e.imageSmoothingEnabled=!1,a){const m=t.clientX-l.left,v=t.clientY-l.top;e.drawImage(this.mainCanvas,m-5,v-5,11,11,d,f,r,r)}else{const m=Math.round(s.x),v=Math.round(s.y);e.drawImage(o,m-5,v-5,11,11,d,f,r,r)}e.restore(),e.strokeStyle="rgba(255,255,255,0.3)",e.lineWidth=.5;const u=r/11;for(let m=0;m<=11;m++){const v=d+m*u,b=f+m*u;e.beginPath(),e.moveTo(v,f),e.lineTo(v,f+r),e.stroke(),e.beginPath(),e.moveTo(d,b),e.lineTo(d+r,b),e.stroke()}const p=d+5*u,_=f+5*u;e.strokeStyle="#fff",e.lineWidth=1.5,e.strokeRect(p,_,u,u),e.strokeStyle="#555",e.lineWidth=1,e.strokeRect(d-.5,f-.5,r+1,c+1),i&&(e.fillStyle=i,e.fillRect(d,f+r+2,n,n),e.fillStyle="#fff",e.font="11px monospace",e.fillText(i.toUpperCase(),d+n+6,f+r+16))}_clearEyedropperPreview(){const t=this.previewCanvas?.getContext("2d");t&&t.clearRect(0,0,this._vw,this._vh)}_renderBrushCursor(){if(!this._pointerOnCanvas||this._altSampling)return;const{activeTool:t}=this.ctx.state,e=this._brushDescriptor,s=e.size,i=e.hardness;if(t!=="pencil"&&t!=="eraser")return;const a=this.previewCanvas.getContext("2d"),o=this._lastPointerScreenX,r=this._lastPointerScreenY,n=s/2*this._zoom;if(a.beginPath(),a.arc(o,r,n,0,Math.PI*2),a.strokeStyle="rgba(0,0,0,0.7)",a.lineWidth=1.5,a.stroke(),a.beginPath(),a.arc(o,r,n,0,Math.PI*2),a.strokeStyle="rgba(255,255,255,0.7)",a.lineWidth=.75,a.stroke(),i<1){const c=n*i;a.beginPath(),a.arc(o,r,c,0,Math.PI*2),a.setLineDash([3,3]),a.strokeStyle="rgba(255,255,255,0.5)",a.lineWidth=.75,a.stroke(),a.setLineDash([])}}_renderStampCursor(){if(!this._pointerOnCanvas||this._transformManager)return;const t=this.ctx.state.stampImage;if(!t||t.naturalWidth<=0||t.naturalHeight<=0)return;const e=this.previewCanvas.getContext("2d"),i=this.ctx.state.stampSize/Math.max(t.naturalWidth,t.naturalHeight),a=Math.max(1,t.naturalWidth*i)*this._zoom,o=Math.max(1,t.naturalHeight*i)*this._zoom,r=this._lastPointerScreenX-a/2,n=this._lastPointerScreenY-o/2;e.save(),e.globalAlpha=.55,e.drawImage(t,r,n,a,o),e.globalAlpha=1,e.strokeStyle="rgba(255,255,255,0.9)",e.lineWidth=1,e.setLineDash([4,3]),e.strokeRect(r-.5,n-.5,a+1,o+1),e.restore()}_renderPreview(){const t=this.previewCanvas?.getContext("2d");if(!t)return;if(this._transformManager){this._transformManager.renderPreview();return}const{activeTool:e}=this.ctx.state;if(e==="pencil"||e==="eraser"||e==="eyedropper"||e==="stamp"){if(t.clearRect(0,0,this._vw,this._vh),this._altSampling||e==="eyedropper")return;e==="stamp"?this._renderStampCursor():this._drawing||this._renderBrushCursor()}}_onPointerDown(t){if(this._invalidateCanvasRect(),this._pointers.set(t.pointerId,{x:t.clientX,y:t.clientY}),this._pointers.size===2){this._enterPinchMode(t);return}if(this._pointers.size>2||!this._ctx.value)return;if(t.button===1){t.preventDefault(),this._startPan(t);return}if(t.button!==0)return;if(this._transformManager){const a=this._getDocPoint(t);t.pointerType==="touch"&&this._transformManager.setTouchMode(!0);const o={shift:t.shiftKey,ctrl:t.ctrlKey||t.metaKey,alt:t.altKey};this._transformManager.onPointerDown(a,o),this.mainCanvas.setPointerCapture(t.pointerId);return}const{activeTool:e}=this.ctx.state;if(t.altKey&&(e==="pencil"||e==="eraser")){this._altSampling=!0;const a=this._getDocPoint(t),o=this._sampleColor(a.x,a.y);o&&this.ctx.setStrokeColor(o);return}if(e==="hand"){this._startPan(t);return}if(e==="crop"){this.mainCanvas.setPointerCapture(t.pointerId);const a=this._getDocPoint(t);this._handleCropPointerDown(a);return}if(e==="eyedropper"){const a=this._getDocPoint(t),o=this._sampleColor(a.x,a.y);o&&this.ctx.setStrokeColor(o);return}const s=this.ctx.state.layers.find(a=>a.id===this.ctx.state.activeLayerId);if(s&&!s.visible)return;if(e==="move"){this.mainCanvas.setPointerCapture(t.pointerId),this._transformManager&&this.commitTransform();const a=this._getDocPoint(t);this._captureBeforeDraw();const o=this._getActiveLayerCtx();if(!o)return;const r=document.createElement("canvas");r.width=this._docWidth,r.height=this._docHeight,r.getContext("2d").drawImage(o.canvas,0,0),this._moveTempCanvas=r,this._moveStartPoint=a;return}this.mainCanvas.setPointerCapture(t.pointerId);const i=this._getDocPoint(t);if(e==="select"){this._handleSelectPointerDown(i);return}if(e==="fill"){const a=Math.round(i.x),o=Math.round(i.y);if(a>=0&&o>=0&&a<this._docWidth&&o<this._docHeight){const r=this._getActiveLayerCtx();r&&(this._captureBeforeDraw(),Ya(r,a,o,this.ctx.state.strokeColor)?(this._pushDrawHistory(),this.composite()):this._beforeDrawCanvas=null)}return}if(e==="stamp"){this._transformManager&&this.commitTransform(),this.ctx.state.stampImage&&(this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this._createStampAsTransform(this.ctx.state.stampImage,i.x,i.y,this.ctx.state.stampSize,t.pointerType==="touch"));return}if(e==="text"){if(t.preventDefault(),this._textEditing){const a=this._getTextBoundingBox();if(i.x>=a.x&&i.x<=a.x+a.w&&i.y>=a.y&&i.y<=a.y+a.h){const o=this._pointToTextOffset(i);this._textAreaEl&&(this._textAreaEl.selectionStart=o,this._textAreaEl.selectionEnd=o),this._textSelectAnchor=o,this._textSelecting=!0,this._startTextCursorBlink(),this._renderTextPreview();return}this._commitText();return}this._textPosition=i,this._textEditing=!0,this._textAreaEl&&(this._textAreaEl.value="",this._textAreaEl.focus()),this._startTextCursorBlink(),this._renderTextPreview();return}if(this._drawing=!0,this._lastPoint=i,this._startPoint=i,e==="pencil"||e==="eraser"){this.previewCanvas?.getContext("2d")?.clearRect(0,0,this._vw,this._vh),this._captureBeforeDraw();const a=this._brushDescriptor,o=this.ctx.state.strokeColor,r=this.ctx.state.activeTool==="eraser";this._engine.begin(a,o,r,this._docWidth,this._docHeight),this._strokeTintNeedsClear=!0;const n=a.ink.wetness>0?this._getActiveLayerCtx()??void 0:void 0;this._engine.stroke(i.x,i.y,vs(t),n,t.timeStamp),this.composite()}}_onPointerMove(t){this._pointers.has(t.pointerId)&&this._pointers.set(t.pointerId,{x:t.clientX,y:t.clientY});const e=this._getCanvasRect();if(this._lastPointerScreenX=t.clientX-e.left,this._lastPointerScreenY=t.clientY-e.top,this._pinching){this._updatePinch();return}if(!this._ctx.value)return;if(this._transformManager){const a=this._getDocPoint(t),o={shift:t.shiftKey,ctrl:t.ctrlKey||t.metaKey,alt:t.altKey},r=this.getTransformValues();this._transformManager.onPointerMove(a,o),this.style.cursor=this._transformManager.getCursor(a),this.scheduleComposite();const n=this.getTransformValues();this._transformValuesEqual(r,n)||this._dispatchTransformChange();return}{const a=this.ctx.state.activeTool;if(a==="eyedropper"){this._renderEyedropperPreview(t);return}if(this._altSampling||t.altKey&&(a==="pencil"||a==="eraser")){if(this._altSampling=t.altKey,!t.altKey){this._clearEyedropperPreview();return}this._renderEyedropperPreview(t);return}}if(this._panning){this._updatePan(t);return}if(this._textSelecting){const a=this._getDocPoint(t),o=this._pointToTextOffset(a);if(this._textAreaEl){const r=this._textSelectAnchor;this._textAreaEl.selectionStart=Math.min(r,o),this._textAreaEl.selectionEnd=Math.max(r,o)}this._renderTextPreview();return}const{activeTool:s}=this.ctx.state;if(s==="crop"){this._handleCropPointerMove(t);return}if(s==="move"&&this._moveTempCanvas&&this._moveStartPoint){const a=this._getDocPoint(t);let o=a.x-this._moveStartPoint.x,r=a.y-this._moveStartPoint.y;t.shiftKey&&(Math.abs(o)>Math.abs(r)?r=0:o=0);const n=this._getActiveLayerCtx();n&&(n.clearRect(0,0,this._docWidth,this._docHeight),n.drawImage(this._moveTempCanvas,Math.round(o),Math.round(r)),this.scheduleComposite());return}if(s==="select"){this._handleSelectPointerMove(t);return}if(s==="stamp"){this._renderPreview();return}if(!this._drawing&&(s==="pencil"||s==="eraser")&&this._renderPreview(),!this._drawing||!this._lastPoint)return;const i=this._getDocPoint(t);if(s==="pencil"||s==="eraser"){const o=this._brushDescriptor.ink.wetness>0?this._getActiveLayerCtx()??void 0:void 0;this._engine.stroke(i.x,i.y,vs(t),o,t.timeStamp),this._lastPoint=i,this.scheduleComposite()}else if(ht(s)){const a=this.previewCanvas.getContext("2d");a.clearRect(0,0,this._vw,this._vh),a.save(),a.translate(this._panX,this._panY),a.scale(this._zoom,this._zoom),cs(a,s,this._startPoint,i,this.ctx.state.strokeColor,this.ctx.state.fillColor,this.ctx.state.useFill,this._brushDescriptor.size),a.restore()}}_onPointerUp(t){if(this._pointers.delete(t.pointerId),this._pinching){this._pointers.size<2&&(this._pinching=!1);return}if(!this._ctx.value)return;if(this._transformManager){const a=this._getDocPoint(t),o=this._transformManager.onPointerUp(a);o==="commit"||o==="commit-button"?this.commitTransform():o==="cancel-button"&&this.cancelTransform(),this.composite();return}if(this._panning){this._endPan();return}if(this._textSelecting){this._textSelecting=!1;return}const{activeTool:e}=this.ctx.state;if(e==="crop"){this._handleCropPointerUp();return}if(this._drawing&&e!=="pencil"&&e!=="eraser"&&!ht(e)){const a=this._getActiveLayerCtx(),o=a?this._commitStroke(a):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!0,o),this.composite();return}if(this._moveTempCanvas&&e!=="move"){this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(!0),this.composite();return}if(e==="move"&&this._moveTempCanvas){this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite();return}if(e==="select"){this._handleSelectPointerUp(t);return}if(!this._drawing)return;const s=this._getDocPoint(t);if(ht(e)){this._captureBeforeDraw();const a=this._getActiveLayerCtx();a&&cs(a,e,this._startPoint,s,this.ctx.state.strokeColor,this.ctx.state.fillColor,this.ctx.state.useFill,this._brushDescriptor.size),this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}let i;if(e==="pencil"||e==="eraser"){const a=this._getActiveLayerCtx();a&&(i=this._commitStroke(a))}this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!1,i),this.composite()}_onPointerLeave(t){if(this._pointerOnCanvas=!1,this._renderPreview(),!this._pinching){try{if(this.mainCanvas.hasPointerCapture(t.pointerId))return}catch{}this._onPointerUp(t)}}_onPointerCancel(t){this._pointers.delete(t.pointerId),this._pinching?this._pointers.size<2&&(this._pinching=!1):this._cancelCurrentTool(t.pointerId)}_cancelCurrentTool(t){try{this.mainCanvas.releasePointerCapture(t)}catch{}if(this._engine.cancel(),this._drawing){if(this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._beforeDrawCanvas){const s=this._getActiveLayerCtx();s&&this._restoreBeforeDraw(s),this._beforeDrawCanvas=null}this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite()}if(this._panning&&this._endPan(),this._moveTempCanvas){if(this._beforeDrawCanvas){const s=this._getActiveLayerCtx();s&&this._restoreBeforeDraw(s),this._beforeDrawCanvas=null}this._moveTempCanvas=null,this._moveStartPoint=null,this.composite()}this._selectionDrawing&&(this._selectionDrawing=!1,this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh));const e=this._cropDragging||this._cropHandle!==null;if(this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,e&&this._cropRect){const s=Pe(this._ctx.value?.state.cropAspectRatio??"free"),i=this._normalizeCropRect(this._cropRect,s);this._cropRect=i.w<1||i.h<1?null:i,this._cropRect?this._drawCropPreview():this._clearCropPreview()}this._updateCropActions()}_enterPinchMode(t){for(const[a]of this._pointers)if(a!==t.pointerId){this._cancelCurrentTool(a);break}this._pinching=!0;const e=[...this._pointers.values()],s=e[1].x-e[0].x,i=e[1].y-e[0].y;this._lastPinchDist=Math.hypot(s,i),this._lastPinchMidX=(e[0].x+e[1].x)/2,this._lastPinchMidY=(e[0].y+e[1].y)/2}_updatePinch(){const t=[...this._pointers.values()];if(t.length<2)return;const e=t[1].x-t[0].x,s=t[1].y-t[0].y,i=Math.hypot(e,s),a=(t[0].x+t[1].x)/2,o=(t[0].y+t[1].y)/2,r=a-this._lastPinchMidX,n=o-this._lastPinchMidY;if(this._panX+=r,this._panY+=n,this._lastPinchDist>0){const c=i/this._lastPinchDist,h=this._getCanvasRect(),l=a-h.left,d=o-h.top,f=(l-this._panX)/this._zoom,u=(d-this._panY)/this._zoom,p=Math.min(R.MAX_ZOOM,Math.max(R.MIN_ZOOM,this._zoom*c));this._panX=l-f*p,this._panY=d-u*p,this._zoom=p}this._lastPinchDist=i,this._lastPinchMidX=a,this._lastPinchMidY=o,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange()}_handleSelectPointerDown(t){this._transformManager&&this.commitTransform(),this._selectionDrawing=!0,this._startPoint=t}_handleCropPointerDown(t){if(this._cropRect){const e=ys(this._cropRect,t,this._zoom);if(e&&e!=="move"){this._cropHandle=e,this._cropDragOrigin={x:t.x,y:t.y},this._cropRectOrigin={...this._cropRect},this._updateCropActions();return}if(e==="move"){this._cropHandle="move",this._cropDragOrigin={x:t.x,y:t.y},this._cropRectOrigin={...this._cropRect},this._updateCropActions();return}}this._cropRect={x:t.x,y:t.y,w:0,h:0},this._cropDragging=!0,this._cropDragOrigin={x:t.x,y:t.y}}_handleSelectPointerMove(t){if(this._selectionDrawing&&this._startPoint){const e=this._getDocPoint(t),s=this.previewCanvas.getContext("2d");s.clearRect(0,0,this._vw,this._vh);const i=Math.min(this._startPoint.x,e.x),a=Math.min(this._startPoint.y,e.y),o=Math.abs(e.x-this._startPoint.x),r=Math.abs(e.y-this._startPoint.y);s.save(),s.translate(this._panX,this._panY),s.scale(this._zoom,this._zoom),Na(s,i,a,o,r,0),s.restore()}}_handleSelectPointerUp(t){if(this._selectionDrawing&&this._startPoint){this._selectionDrawing=!1;const e=this._getDocPoint(t),s=Math.min(this._startPoint.x,e.x),i=Math.min(this._startPoint.y,e.y),a=Math.max(this._startPoint.x,e.x),o=Math.max(this._startPoint.y,e.y);this._startPoint=null;const r=Math.max(0,Math.min(this._docWidth,s)),n=Math.max(0,Math.min(this._docHeight,i)),c=Math.max(0,Math.min(this._docWidth,a)),h=Math.max(0,Math.min(this._docHeight,o)),l=c-r,d=h-n;if(l<2||d<2){this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh);return}const f=Math.round(r),u=Math.round(n),p=Math.round(c)-f,_=Math.round(h)-u;if(p<1||_<1){this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh);return}const m=this._ctx.value?.state;if(!m)return;const v=m.layers.find(b=>b.id===m.activeLayerId);if(v&&p>0&&_>0){const b=v.canvas.getContext("2d");this._captureBeforeDraw();const y=b.getImageData(f,u,p,_);b.clearRect(f,u,p,_),this._transformContentMode="lifted",this._transformManager=new rt(y,{x:f,y:u,w:p,h:_},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange()}}}_handleCropPointerMove(t){const e=this._getDocPoint(t),s=Pe(this.ctx.state.cropAspectRatio);if(this._cropDragging&&this._cropDragOrigin){let i={x:this._cropDragOrigin.x,y:this._cropDragOrigin.y,w:e.x-this._cropDragOrigin.x,h:e.y-this._cropDragOrigin.y};s&&(i=ws(i,s,"draw")),this._cropRect=i,this._drawCropPreview();return}if(this._cropHandle&&this._cropDragOrigin&&this._cropRectOrigin){const i=e.x-this._cropDragOrigin.x,a=e.y-this._cropDragOrigin.y,o=this._cropRectOrigin;if(this._cropHandle==="move"){let r=o.x+i,n=o.y+a;const c=Math.abs(o.w),h=Math.abs(o.h);r=Math.max(0,Math.min(r,this._docWidth-c)),n=Math.max(0,Math.min(n,this._docHeight-h)),this._cropRect={x:r,y:n,w:c,h}}else{let r=this._resizeCropRect(o,this._cropHandle,i,a);s&&(r=ws(r,s,this._cropHandle)),this._cropRect=r}this._drawCropPreview();return}if(this._cropRect){const i=ys(this._cropRect,e,this._zoom);i&&i!=="move"?this.mainCanvas.style.cursor=this._cropHandleCursor(i):i==="move"?this.mainCanvas.style.cursor="move":this.mainCanvas.style.cursor="crosshair"}}_cropHandleCursor(t){return{nw:"nwse-resize",n:"ns-resize",ne:"nesw-resize",e:"ew-resize",se:"nwse-resize",s:"ns-resize",sw:"nesw-resize",w:"ew-resize"}[t]??"crosshair"}_resizeCropRect(t,e,s,i){let{x:a,y:o,w:r,h:n}=t;switch(e){case"nw":a+=s,o+=i,r-=s,n-=i;break;case"n":o+=i,n-=i;break;case"ne":r+=s,o+=i,n-=i;break;case"e":r+=s;break;case"se":r+=s,n+=i;break;case"s":n+=i;break;case"sw":a+=s,r-=s,n+=i;break;case"w":a+=s,r-=s;break}return{x:a,y:o,w:r,h:n}}_handleCropPointerUp(){const t=Pe(this.ctx.state.cropAspectRatio);this._cropDragging&&this._cropRect&&(this._cropRect=this._normalizeCropRect(this._cropRect,t),(this._cropRect.w<1||this._cropRect.h<1)&&(this._cropRect=null)),this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._cropRect&&(this._cropRect=this._normalizeCropRect(this._cropRect,t),(this._cropRect.w<1||this._cropRect.h<1)&&(this._cropRect=null)),this._updateCropActions(),this._cropRect?this._drawCropPreview():this._clearCropPreview()}_normalizeCropRect(t,e){let{x:s,y:i,w:a,h:o}=t;a<0&&(s+=a,a=-a),o<0&&(i+=o,o=-o),e===void 0&&o>0&&a>0&&(e=a/o),s<0&&(a+=s,s=0),i<0&&(o+=i,i=0);const r=this._docWidth-s,n=this._docHeight-i,c=a>r,h=o>n;if(a=Math.min(a,r),o=Math.min(o,n),e&&(c||h)){if(c&&h){const p=a/e,_=o*e;p<=n?o=p:_<=r?a=_:a/o>e?a=o*e:o=a/e}else c?o=a/e:a=o*e;a=Math.min(a,this._docWidth-s),o=Math.min(o,this._docHeight-i)}const l=Math.round(s),d=Math.round(i);let f=Math.round(a),u=Math.round(o);return f=Math.min(f,this._docWidth-l),u=Math.min(u,this._docHeight-d),{x:l,y:d,w:f,h:u}}_drawCropPreview(){if(!this.previewCanvas||!this._cropRect)return;const t=this.previewCanvas.getContext("2d");t.clearRect(0,0,this._vw,this._vh),t.save(),t.translate(this._panX,this._panY),t.scale(this._zoom,this._zoom),Fa(t,this._cropRect,this._docWidth,this._docHeight,this._zoom),t.restore()}commitCrop(){if(!this._cropRect)return;const t=this._cropRect;if(t.w<1||t.h<1)return;const e=this._ctx.value?.state;if(!e)return;const s=this._docWidth,i=this._docHeight,a=e.layers.map(r=>{const n=r.canvas.getContext("2d");return{id:r.id,name:r.name,visible:r.visible,opacity:r.opacity,blendMode:r.blendMode,imageData:n.getImageData(0,0,r.canvas.width,r.canvas.height)}});for(const r of e.layers){const c=r.canvas.getContext("2d").getImageData(t.x,t.y,t.w,t.h),h=document.createElement("canvas");h.width=t.w,h.height=t.h,h.getContext("2d").putImageData(c,0,0),r.canvas=h}const o=e.layers.map(r=>{const n=r.canvas.getContext("2d");return{id:r.id,name:r.name,visible:r.visible,opacity:r.opacity,blendMode:r.blendMode,imageData:n.getImageData(0,0,r.canvas.width,r.canvas.height)}});this.dispatchEvent(new CustomEvent("crop-commit",{bubbles:!0,composed:!0,detail:{width:t.w,height:t.h}})),this._pushHistoryEntry({type:"crop",beforeLayers:a,afterLayers:o,beforeWidth:s,beforeHeight:i,afterWidth:t.w,afterHeight:t.h}),this._cropRect=null,this._clearCropPreview(),this.composite()}cancelCrop(){this._cropRect&&(this._cropRect=null,this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._clearCropPreview())}get hasCropRect(){return this._cropRect!==null}_clearCropPreview(){this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}_createStampAsTransform(t,e,s,i,a=!1){if(t.naturalWidth<=0||t.naturalHeight<=0)return;const o=i/Math.max(t.naturalWidth,t.naturalHeight),r=Math.max(1,Math.round(t.naturalWidth*o)),n=Math.max(1,Math.round(t.naturalHeight*o)),c=Math.round(e-r/2),h=Math.round(s-n/2),l=document.createElement("canvas");l.width=r,l.height=n,l.getContext("2d").drawImage(t,0,0,r,n);const d=l.getContext("2d").getImageData(0,0,r,n);this._transformContentMode="inserted",this._transformManager=new rt(d,{x:c,y:h,w:r,h:n},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this._transformManager.setTouchMode(a),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}async _handleExternalImage(t,e){this._transformManager&&this.commitTransform();let s=t.naturalWidth,i=t.naturalHeight;const a=this._docWidth,o=this._docHeight;if((s>a||i>o)&&await this._resizeDialog.show(s,i,a,o)){const u=Math.min(a/s,o/i);s=Math.round(s*u),i=Math.round(i*u)}this.ctx.addLayer(e),await this.updateComplete,this._captureBeforeDraw(),this._floatIsExternalImage=!0;const r=this._docWidth/2,n=this._docHeight/2,c=Math.round(r-s/2),h=Math.round(n-i/2),l=document.createElement("canvas");l.width=s,l.height=i,l.getContext("2d").drawImage(t,0,0,s,i);const d=l.getContext("2d").getImageData(0,0,s,i);this._transformContentMode="inserted",this._transformManager=new rt(d,{x:c,y:h,w:s,h:i},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}copySelection(){if(!this._transformManager)return;const t=this._transformManager.snapshot(),e=t.canvas.getContext("2d");this.commitTransform(),this._clipboard=e.getImageData(0,0,t.w,t.h),this._clipboardOrigin={x:t.x,y:t.y},this._clipboardRotation=0,this._writeToSystemClipboard(t.canvas),this._notifyHistory()}_writeToSystemClipboard(t){t.toBlob(e=>{e&&(this._clipboardBlobSize=e.size,!(!navigator.clipboard?.write||typeof ClipboardItem>"u")&&navigator.clipboard.write([new ClipboardItem({"image/png":e})]).catch(()=>{}))},"image/png")}cutSelection(){if(!this._transformManager)return;this.copySelection();const t=this._clipboardOrigin,e=this._clipboard;if(t&&e){this._captureBeforeDraw();const s=this._getActiveLayerCtx();s?(s.clearRect(t.x,t.y,e.width,e.height),this._pushDrawHistory(!0),this.composite()):this._beforeDrawCanvas=null}}pasteSelection(){if(!this._clipboard||!this._clipboardOrigin)return;this._transformManager&&this.commitTransform(),this._beforeDrawCanvas||this._captureBeforeDraw();const t=this._clipboard.width,e=this._clipboard.height,s=Math.max(0,Math.min(this._clipboardOrigin.x,this._docWidth-1)),i=Math.max(0,Math.min(this._clipboardOrigin.y,this._docHeight-1)),a=new ImageData(new Uint8ClampedArray(this._clipboard.data),t,e);this._transformContentMode="inserted",this._transformManager=new rt(a,{x:s,y:i,w:t,h:e},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this._clipboardRotation&&(this._transformManager.rotation=this._clipboardRotation*180/Math.PI),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}async paste(){try{const t=await navigator.clipboard.read();for(const e of t){const s=e.types.find(r=>r.startsWith("image/"));if(!s)continue;const i=await e.getType(s),a=URL.createObjectURL(i);let o;try{o=await new Promise((r,n)=>{const c=new Image;c.onload=()=>r(c),c.onerror=()=>n(new Error("Image load failed")),c.src=a}),URL.revokeObjectURL(a)}catch{URL.revokeObjectURL(a);continue}if(this._clipboard&&o.naturalWidth===this._clipboard.width&&o.naturalHeight===this._clipboard.height){this.pasteSelection();return}await this._handleExternalImage(o,"Pasted Image");return}}catch{}this.pasteSelection()}selectAll(){this._transformManager&&this.commitTransform();const t=this._ctx.value?.state;if(!t)return;const e=t.layers.find(c=>c.id===t.activeLayerId);if(!e)return;const s=e.canvas.getContext("2d"),i=this._docWidth,a=this._docHeight,o=s.getImageData(0,0,i,a),r=ks(o);if(!r)return;this._captureBeforeDraw();const n=s.getImageData(r.x,r.y,r.w,r.h);s.clearRect(r.x,r.y,r.w,r.h),this._transformContentMode="lifted",this._transformManager=new rt(n,r,this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange()}selectAllCanvas(){this._transformManager&&this.commitTransform();const t=this._ctx.value?.state;if(!t)return;const e=t.layers.find(r=>r.id===t.activeLayerId);if(!e)return;const s=e.canvas.getContext("2d"),i=this._docWidth,a=this._docHeight;this._captureBeforeDraw();const o=s.getImageData(0,0,i,a);s.clearRect(0,0,i,a),this._transformContentMode="lifted",this._transformManager=new rt(o,{x:0,y:0,w:i,h:a},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange()}duplicateInPlace(){if(this._transformManager){const t=this._transformManager.snapshot(),e=t.canvas.getContext("2d").getImageData(0,0,t.w,t.h);this._clipboard=new ImageData(new Uint8ClampedArray(e.data),e.width,e.height),this._clipboardOrigin={x:t.x,y:t.y},this._clipboardRotation=0,this._writeToSystemClipboard(t.canvas),this.commitTransform(),this._captureBeforeDraw();const s=new ImageData(new Uint8ClampedArray(e.data),e.width,e.height);this._transformContentMode="inserted",this._transformManager=new rt(s,{x:t.x,y:t.y,w:t.w,h:t.h},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}else this.pasteSelection()}deleteSelection(){if(this._transformManager){if(this._transformContentMode==="inserted"&&!this._beforeDrawCanvas){this.cancelTransform();return}this._beforeDrawCanvas||this._captureBeforeDraw(),this._transformManager.dispose(),this._transformManager=null,this._transformContentMode="lifted",this._pushDrawHistory(!0),this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}}clearSelection(){if(this._textEditing&&this._commitText(),this._cropRect&&this.cancelCrop(),this._drawing){const t=this._getActiveLayerCtx(),e=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!1,e),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite()}this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._selectionDrawing&&(this._selectionDrawing=!1,this._startPoint=null,this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)),this._transformManager&&this.commitTransform()}cancelExternalFloat(){if(!this._floatIsExternalImage||!this._transformManager)return;const t=this.ctx.state.activeLayerId;if(this._transformManager.dispose(),this._transformManager=null,this._floatIsExternalImage=!1,this._transformContentMode="lifted",this._selectionDrawing=!1,this._beforeDrawCanvas=null,this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this._dispatchTransformChange(),this._historyIndex>=0){let e=-1;for(let s=this._historyIndex;s>=0;s--){const i=this._history[s];if(i.type==="add-layer"&&i.layer.id===t){e=s;break}}if(e>=0){const s=this._history.slice(0,e),a=this._history.slice(e,this._historyIndex+1).filter(o=>{const r=this._getEntryLayerId(o);return r===null||r!==t});this._history=[...s,...a],this._historyIndex=this._history.length-1}}this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:t}})),this.composite(),this._notifyHistory()}get hasExternalFloat(){return this._floatIsExternalImage&&this._transformManager!==null}getFloatSnapshot(){if(!this._transformManager)return null;const t=this._ctx.value?.state.activeLayerId;if(!t)return null;const e=this._transformManager.snapshot();return{layerId:t,tempCanvas:e.canvas,x:e.x,y:e.y}}connectedCallback(){super.connectedCallback();const t=document.createElement("textarea");t.style.cssText="position:fixed;top:0;left:0;width:1px;height:1px;opacity:0;border:0;padding:0;margin:0;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;",t.setAttribute("autocomplete","off"),t.setAttribute("autocorrect","off"),t.setAttribute("autocapitalize","off"),t.setAttribute("spellcheck","false"),t.setAttribute("aria-label","Text"),t.addEventListener("input",()=>{this._textEditing&&(this._startTextCursorBlink(),this._renderTextPreview(),this._dispatchPendingTextChange())}),t.addEventListener("keydown",e=>this._onTextKeydown(e)),t.addEventListener("selectionchange",()=>{this._textEditing&&(this._startTextCursorBlink(),this._renderTextPreview())}),this._textAreaEl=t,this.addEventListener("wheel",this._onWheel,{passive:!1}),this.addEventListener("dragover",this._onDragOver),this.addEventListener("dragenter",this._onDragEnter),this.addEventListener("dragleave",this._onDragLeave),this.addEventListener("drop",this._onDrop),window.addEventListener("blur",this._onWindowBlur),window.addEventListener("resize",this._invalidateCanvasRect),window.addEventListener("scroll",this._invalidateCanvasRect,!0)}disconnectedCallback(){super.disconnectedCallback(),this._textCursorInterval&&(clearInterval(this._textCursorInterval),this._textCursorInterval=0),this._textAreaEl&&(this._textAreaEl.remove(),this._textAreaEl=null),this._resizeObserver?.disconnect(),this._resizeObserver=null,this._compositeScheduler.cancel(),this._transformManager&&(this._transformManager.dispose(),this._transformManager=null,this._transformContentMode="lifted",this._floatIsExternalImage=!1),this._pointers.clear(),this._pinching=!1,this._panning=!1,this._panPointerId=-1,this.removeEventListener("wheel",this._onWheel),this.removeEventListener("dragover",this._onDragOver),this.removeEventListener("dragenter",this._onDragEnter),this.removeEventListener("dragleave",this._onDragLeave),this.removeEventListener("drop",this._onDrop),window.removeEventListener("blur",this._onWindowBlur),window.removeEventListener("resize",this._invalidateCanvasRect),window.removeEventListener("scroll",this._invalidateCanvasRect,!0)}_renderTextPreview(){if(!this._textEditing||!this._textAreaEl||!this.previewCanvas)return;const t=this.previewCanvas.getContext("2d");t.clearRect(0,0,this._vw,this._vh);const e=this.ctx.state,s=this._textAreaEl.value,{fontFamily:i,fontSize:a,fontBold:o,fontItalic:r,strokeColor:n}=e;t.save(),t.translate(this._panX,this._panY),t.scale(this._zoom,this._zoom),xs(t,s,this._textPosition.x,this._textPosition.y,a,i,o,r,n);const c=Cs(t,s,a,i,o,r),h=a*le,l=this._textAreaEl.selectionStart??0,d=this._textAreaEl.selectionEnd??l,f=s.split(`
`);t.font=he(a,i,o,r),t.textBaseline="top";const u=y=>{let x=0;for(let k=0;k<f.length;k++){if(x+f[k].length>=y){const T=y-x,S=f[k].substring(0,T);return{line:k,x:this._textPosition.x+t.measureText(S).width,y:this._textPosition.y+k*h}}x+=f[k].length+1}const C=f.length-1;return{line:C,x:this._textPosition.x+t.measureText(f[C]).width,y:this._textPosition.y+C*h}};if(l!==d){t.fillStyle="rgba(99, 102, 241, 0.3)";const y=u(l),x=u(d);if(y.line===x.line)t.fillRect(y.x,y.y,x.x-y.x,h);else{const C=t.measureText(f[y.line]).width;t.fillRect(y.x,y.y,this._textPosition.x+C-y.x,h);for(let k=y.line+1;k<x.line;k++){const T=t.measureText(f[k]).width;t.fillRect(this._textPosition.x,this._textPosition.y+k*h,T,h)}t.fillRect(this._textPosition.x,x.y,x.x-this._textPosition.x,h)}}else if(this._textCursorVisible){const y=u(l);t.fillStyle=n,t.fillRect(y.x,y.y,2/this._zoom,a)}const p=4/this._zoom,_=this._textPosition.x-p,m=this._textPosition.y-p,v=Math.max(c.width,a)+p*2,b=c.height+p*2;t.strokeStyle="rgba(99, 102, 241, 0.6)",t.lineWidth=1/this._zoom,t.setLineDash([4/this._zoom,4/this._zoom]),t.strokeRect(_,m,v,b),t.restore()}_getTextBoundingBox(){const t=this.ctx.state,e=this._textAreaEl?.value??"",s=this.previewCanvas.getContext("2d"),i=Cs(s,e,t.fontSize,t.fontFamily,t.fontBold,t.fontItalic),a=4/this._zoom;return{x:this._textPosition.x-a,y:this._textPosition.y-a,w:Math.max(i.width,t.fontSize)+a*2,h:i.height+a*2}}_pointToTextOffset(t){if(!this._textAreaEl)return 0;const e=this.ctx.state,i=this._textAreaEl.value.split(`
`),{fontSize:a,fontFamily:o,fontBold:r,fontItalic:n}=e,c=a*le,h=this.previewCanvas.getContext("2d");h.save(),h.font=he(a,o,r,n),h.textBaseline="top";const l=t.y-this._textPosition.y;let d=Math.floor(l/c);d=Math.max(0,Math.min(d,i.length-1));const f=i[d],u=t.x-this._textPosition.x;let p=0;for(let m=0;m<=f.length;m++){const v=h.measureText(f.substring(0,m)).width;if(m>0){const y=(h.measureText(f.substring(0,m-1)).width+v)/2;u>=y&&(p=m)}else u<0&&(p=0)}h.restore();let _=0;for(let m=0;m<d;m++)_+=i[m].length+1;return _+p}_startTextCursorBlink(){this._textCursorVisible=!0,this._textCursorInterval&&clearInterval(this._textCursorInterval),this._textCursorInterval=window.setInterval(()=>{this._textCursorVisible=!this._textCursorVisible,this._renderTextPreview()},530)}_stopTextCursorBlink(){this._textCursorInterval&&(clearInterval(this._textCursorInterval),this._textCursorInterval=0),this._textCursorVisible=!1}_onTextKeydown(t){this._textEditing&&(t.key==="Escape"?(t.preventDefault(),t.stopPropagation(),this._textAreaEl&&this._textAreaEl.value.length>0?this._commitText():this._cancelText()):t.key==="Tab"&&t.preventDefault())}_commitText(){if(!this._textEditing||!this._textAreaEl)return;const t=this._textAreaEl.value;if(!t){this._cancelText();return}const e=this.ctx.state;this._captureBeforeDraw();const s=this._getActiveLayerCtx();s?(xs(s,t,this._textPosition.x,this._textPosition.y,e.fontSize,e.fontFamily,e.fontBold,e.fontItalic,e.strokeColor),this._pushDrawHistory(),this.composite()):this._beforeDrawCanvas=null,this._endTextEditing()}_cancelText(){this._endTextEditing()}_endTextEditing(){this._textEditing=!1,this._stopTextCursorBlink(),this._textAreaEl&&(this._textAreaEl.value="",this._textAreaEl.blur()),this._dispatchPendingTextChange(),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}render(){return g`
      <canvas
        id="main"
        @pointerenter=${this._onPointerEnter}
        @pointerdown=${this._onPointerDown}
        @pointermove=${this._onPointerMove}
        @pointerup=${this._onPointerUp}
        @pointerleave=${this._onPointerLeave}
        @pointercancel=${this._onPointerCancel}
      ></canvas>
      <canvas
        id="preview"
        style="position:absolute;top:0;left:0;pointer-events:none;"
      ></canvas>
      ${this._cropActionsVisible?g`
        <div class="crop-actions" role="group" aria-label="Crop actions">
          <button class="apply" type="button" title="Apply crop (Enter)" @click=${this._onApplyCropClick}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
            Apply
          </button>
          <button type="button" title="Cancel crop (Esc)" @click=${this._onCancelCropClick}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
            Cancel
          </button>
        </div>
      `:""}
      <resize-dialog></resize-dialog>
    `}};R.styles=bt`
    :host {
      display: block;
      flex: 1;
      overflow: hidden;
      position: relative;
      background: #3a3a3a;
    }

    canvas {
      display: block;
      touch-action: none;
    }

    #main {
      background: transparent;
      cursor: crosshair;
    }

    :host(.drop-target) #main {
      outline: 3px dashed #4a90d9;
      outline-offset: -3px;
    }

    /* Crop has no keyboard on touch devices, so the commit/cancel actions
       live on the canvas instead of behind Enter/Escape. */
    .crop-actions {
      position: absolute;
      left: 50%;
      bottom: 1rem;
      transform: translateX(-50%);
      display: flex;
      gap: 0.5rem;
      padding: 0.375rem;
      border-radius: 0.625rem;
      background: rgba(30, 30, 30, 0.92);
      border: 1px solid #555;
      box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
    }

    .crop-actions button {
      display: inline-flex;
      align-items: center;
      gap: 0.375rem;
      min-height: 44px;
      padding: 0 0.875rem;
      border: 1px solid #555;
      border-radius: 0.5rem;
      background: #3a3a3a;
      color: #ddd;
      font-family: inherit;
      font-size: 0.875rem;
      cursor: pointer;
      touch-action: manipulation;
    }

    .crop-actions button.apply {
      background: #2f7d32;
      border-color: #3f9e43;
      color: #fff;
    }
  `;R.MIN_ZOOM=.1;R.MAX_ZOOM=10;R.ZOOM_STEP=1.1;Zt([fe("#main")],R.prototype,"mainCanvas",2);Zt([fe("#preview")],R.prototype,"previewCanvas",2);Zt([fe("resize-dialog")],R.prototype,"_resizeDialog",2);Zt([M()],R.prototype,"_cropActionsVisible",2);R=Zt([yt("drawing-canvas")],R);var co=Object.defineProperty,lo=Object.getOwnPropertyDescriptor,at=(t,e,s,i)=>{for(var a=i>1?void 0:i?lo(e,s):e,o=t.length-1,r;o>=0;o--)(r=t[o])&&(a=(i?r(e,s,a):r(a))||a);return i&&a&&co(e,s,a),a};let V=class extends F{constructor(){super(...arguments),this._ctx=new gt(this,{context:Et,subscribe:!0}),this._floatDetail=null,this._thumbnailScheduler=Ne(()=>this._updateThumbnails(),250),this._onComposited=t=>{this._floatDetail=t.detail,this._thumbnailScheduler.schedule()},this._onDocClick=()=>{this._closeContextMenu(),this._dropdownOpen=!1},this._onDocKeyDown=t=>{t.key==="Escape"&&(this._closeContextMenu(),this._dropdownOpen=!1)},this._sheetOpen=!1,this._sheetY=0,this._sheetDragging=!1,this._syncingSheet=!1,this._sheetDragStartY=0,this._sheetDragStartTranslate=0,this._sheetSnapHalf=0,this._sheetSnapFull=0,this._sheetDragTimestamps=[],this._contextMenuOpen=!1,this._contextMenuX=0,this._contextMenuY=0,this._dropdownOpen=!1,this._editingLayerId=null,this._draggedLayerId=null,this._dragPointerId=null,this._dragStartY=0,this._dragCurrentY=0,this._dragThreshold=5,this._dragActivated=!1,this._opacityBefore=null,this._eyeOpen=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>`,this._eyeClosed=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/></svg>`,this._plusIcon=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>`,this._trashIcon=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>`,this._dragTarget=null,this._onResize=()=>{this._sheetOpen&&!this._sheetDragging&&this._recalcSnapPoints()},this._lastThumbLayers=null,this._lastThumbRowCount=-1,this._miniCheckerboardPattern=null}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),this.getRootNode().addEventListener("composited",this._onComposited),window.addEventListener("resize",this._onResize),document.addEventListener("click",this._onDocClick),document.addEventListener("keydown",this._onDocKeyDown)}disconnectedCallback(){super.disconnectedCallback(),this.getRootNode().removeEventListener("composited",this._onComposited),this._thumbnailScheduler.cancel(),window.removeEventListener("resize",this._onResize),document.removeEventListener("click",this._onDocClick),document.removeEventListener("keydown",this._onDocKeyDown)}_toggleVisibility(t,e){e.stopPropagation(),this.ctx.setLayerVisibility(t.id,!t.visible)}_startRename(t,e){e.stopPropagation(),this._editingLayerId=t}_commitRename(t,e){if(this._editingLayerId!==t)return;const s=e.value.trim();s&&s!==this._getLayerById(t)?.name&&this.ctx.renameLayer(t,s),this._editingLayerId=null}_onRenameKeyDown(t,e){e.stopPropagation(),e.key==="Enter"?this._commitRename(t,e.target):e.key==="Escape"&&(this._editingLayerId=null)}_onRenameBlur(t,e){this._commitRename(t,e.target)}_moveUp(t,e){e.stopPropagation();const s=this.ctx.state.layers,i=s.findIndex(a=>a.id===t.id);i<s.length-1&&this.ctx.reorderLayer(t.id,i+1)}_moveDown(t,e){e.stopPropagation();const i=this.ctx.state.layers.findIndex(a=>a.id===t.id);i>0&&this.ctx.reorderLayer(t.id,i-1)}_onReorderPointerDown(t,e){e.button===0&&(this._draggedLayerId=t.id,this._dragPointerId=e.pointerId,this._dragStartY=e.clientY,this._dragCurrentY=e.clientY,this._dragActivated=!1,this._dragTarget=e.currentTarget)}_onReorderPointerMove(t){if(this._dragPointerId!==t.pointerId||!this._draggedLayerId)return;if(this._dragCurrentY=t.clientY,!this._dragActivated){if(Math.abs(this._dragCurrentY-this._dragStartY)<this._dragThreshold)return;this._dragActivated=!0,this._dragTarget&&this._dragTarget.setPointerCapture(t.pointerId)}this._clearDropIndicators();const e=this.shadowRoot?.querySelectorAll(".layer-row");if(e)for(const s of e){const i=s.getBoundingClientRect();if(this._dragCurrentY>=i.top&&this._dragCurrentY<=i.bottom){const a=i.top+i.height/2;this._dragCurrentY<a?s.classList.add("drop-above"):s.classList.add("drop-below");break}}}_onReorderPointerUp(t){if(this._dragPointerId!==t.pointerId)return;const e=this._draggedLayerId;if(!e||!this._dragActivated){this._clearDragState();return}const s=this.shadowRoot?.querySelectorAll(".layer-row");if(!s){this._clearDragState();return}let i=null,a=!1;for(const o of s){const r=o.getBoundingClientRect();if(this._dragCurrentY>=r.top&&this._dragCurrentY<=r.bottom){i=o.dataset.layerId??null;const n=r.top+r.height/2;a=this._dragCurrentY<n;break}}if(i&&i!==e){const o=this.ctx.state.layers,r=o.findIndex(n=>n.id===i);if(r!==-1){let n=a?r+1:r;const c=o.findIndex(h=>h.id===e);c<n&&(n-=1),n=Math.max(0,Math.min(o.length-1,n)),c!==n&&this.ctx.reorderLayer(e,n)}}this._clearDragState()}_onReorderPointerCancel(t){this._clearDragState()}_clearDropIndicators(){this.shadowRoot?.querySelectorAll(".layer-row")?.forEach(e=>e.classList.remove("drop-above","drop-below"))}_clearDragState(){this._draggedLayerId=null,this._dragPointerId=null,this._dragActivated=!1,this._dragTarget=null,this._clearDropIndicators()}openSheet(){this._sheetSnapFull=0,this._sheetY=0,this._sheetOpen=!0,this.updateComplete.then(()=>this._measureSnaps())}_measureSnaps(){const t=this.shadowRoot?.querySelector(".sheet"),e=t?t.offsetHeight:window.innerHeight*.9;this._sheetSnapHalf=Math.floor(e*.5)}closeSheet(){this._sheetOpen=!1,this._sheetY=0,!this._syncingSheet&&this.ctx?.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}_recalcSnapPoints(){const t=this.shadowRoot?.querySelector(".sheet"),e=t?t.offsetHeight:window.innerHeight*.9,s=this._sheetSnapHalf;if(this._sheetSnapFull=0,this._sheetSnapHalf=Math.floor(e*.5),s>0){if(this._sheetY===s)this._sheetY=this._sheetSnapHalf;else if(this._sheetY!==this._sheetSnapFull){const i=Math.abs(this._sheetY-this._sheetSnapHalf),a=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=i<a?this._sheetSnapHalf:this._sheetSnapFull}}}_onSheetHandlePointerDown(t){this._sheetDragging=!0,this._sheetDragStartY=t.clientY,this._sheetDragStartTranslate=this._sheetY,this._sheetDragTimestamps=[{y:t.clientY,t:Date.now()}],t.target.setPointerCapture(t.pointerId)}_onSheetHandlePointerMove(t){if(!this._sheetDragging)return;const e=t.clientY-this._sheetDragStartY,s=Math.max(this._sheetSnapFull,this._sheetDragStartTranslate+e);this._sheetY=s,this._sheetDragTimestamps.push({y:t.clientY,t:Date.now()}),this._sheetDragTimestamps.length>5&&this._sheetDragTimestamps.shift()}_onSheetHandlePointerUp(t){if(!this._sheetDragging)return;this._sheetDragging=!1;const e=this._sheetDragTimestamps;let s=0;if(e.length>=2){const r=e[e.length-1],n=e[e.length-2],c=r.t-n.t;c>0&&(s=(r.y-n.y)/c)}const i=window.innerHeight*.75;if(this._sheetY>i||s>.5){this.closeSheet();return}const a=Math.abs(this._sheetY-this._sheetSnapHalf),o=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=a<o?this._sheetSnapHalf:this._sheetSnapFull}_onSheetHandlePointerCancel(t){if(!this._sheetDragging)return;this._sheetDragging=!1;const e=Math.abs(this._sheetY-this._sheetSnapHalf),s=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=e<s?this._sheetSnapHalf:this._sheetSnapFull}_onOpacityPointerDown(t){this._opacityBefore=t.opacity}_onOpacityInput(t,e){if(this._opacityBefore===null){const i=this._getLayerById(t);i&&(this._opacityBefore=i.opacity)}const s=Number(e.target.value)/100;this.ctx.setLayerOpacity(t,s)}_onOpacityChange(t,e){const s=Number(e.target.value)/100,i=this._opacityBefore;this._opacityBefore=null,i!==null&&i!==s&&this.dispatchEvent(new CustomEvent("commit-opacity",{bubbles:!0,composed:!0,detail:{layerId:t,before:i,after:s}}))}_onContextMenu(t,e){t.preventDefault(),this._selectLayer(e),this._contextMenuX=t.clientX,this._contextMenuY=t.clientY,this._contextMenuOpen=!0}_closeContextMenu(){this._contextMenuOpen=!1}_getLayerById(t){return this.ctx.state.layers.find(e=>e.id===t)}_selectLayer(t){this.ctx.setActiveLayer(t)}render(){if(!this._ctx.value)return g``;if(this.ctx.isMobile)return this._renderMobileSheet();const{layersPanelOpen:t}=this.ctx.state;return t?g`
      <div class="panel">
        ${this._renderLayersList()}
      </div>
    `:g`
        <div class="panel collapsed">
          <div class="collapsed-strip">
            <button
              class="expand-btn"
              title="Show layers"
              @click=${()=>this.ctx.toggleLayersPanel()}
            >&#9654;</button>
            <span class="vertical-label">Layers</span>
          </div>
        </div>
      `}_renderMobileSheet(){const t=this._sheetOpen?`transform: translateY(${this._sheetY}px);${this._sheetDragging?"transition:none;":""}`:"transform: translateY(100%);";return g`
      <div
        class="sheet-backdrop ${this._sheetOpen?"open":""}"
        @click=${()=>this.closeSheet()}
      ></div>
      <div
        class="sheet ${this._sheetOpen?"open":""}"
        style=${t}
      >
        <div
          class="sheet-handle"
          @pointerdown=${e=>this._onSheetHandlePointerDown(e)}
          @pointermove=${e=>this._onSheetHandlePointerMove(e)}
          @pointerup=${e=>this._onSheetHandlePointerUp(e)}
          @pointercancel=${e=>this._onSheetHandlePointerCancel(e)}
        >
          <div class="sheet-handle-bar"></div>
        </div>
        <div class="sheet-content">
          ${this._renderLayersList()}
        </div>
      </div>
    `}_renderLayersList(){const{layers:t,activeLayerId:e}=this.ctx.state,s=[...t].reverse();return g`
      <div class="header">
        <span class="header-title">Layers</span>
        <button
          class="collapse-btn"
          title="Hide layers"
          @click=${()=>this.ctx.toggleLayersPanel()}
        >&#9664; hide</button>
      </div>

      <div class="layer-list">
        ${s.map(i=>this._renderLayerRow(i,t,e))}
      </div>

      ${this._contextMenuOpen?g`
        <div
          class="context-menu"
          style="left:${this._contextMenuX}px;top:${this._contextMenuY}px"
          @click=${i=>i.stopPropagation()}
        >
          <button
            class="context-menu-item"
            ?disabled=${t.findIndex(i=>i.id===e)===0}
            @click=${()=>{this.ctx.mergeLayerDown(e),this._closeContextMenu()}}
          >Merge Down</button>
          <button
            class="context-menu-item"
            ?disabled=${t.filter(i=>i.visible).length<2}
            @click=${()=>{this.ctx.mergeVisibleLayers(),this._closeContextMenu()}}
          >Merge Visible</button>
          <button
            class="context-menu-item"
            ?disabled=${t.length<=1}
            @click=${()=>{this.ctx.flattenImage(),this._closeContextMenu()}}
          >Flatten Image</button>
        </div>
      `:P}

      <div class="action-bar-wrapper">
        ${this._dropdownOpen?g`
          <div class="dropdown-menu" @click=${i=>i.stopPropagation()}>
            <button
              ?disabled=${t.findIndex(i=>i.id===e)===0}
              @click=${()=>{this.ctx.mergeLayerDown(e),this._dropdownOpen=!1}}
            >Merge Down</button>
            <button
              ?disabled=${t.filter(i=>i.visible).length<2}
              @click=${()=>{this.ctx.mergeVisibleLayers(),this._dropdownOpen=!1}}
            >Merge Visible</button>
            <button
              ?disabled=${t.length<=1}
              @click=${()=>{this.ctx.flattenImage(),this._dropdownOpen=!1}}
            >Flatten Image</button>
          </div>
        `:P}
        <div class="action-bar">
          <button
            class="action-btn"
            title="Add layer"
            @click=${()=>this.ctx.addLayer()}
          >${this._plusIcon} Add</button>
          <button
            class="action-btn"
            title="Delete layer"
            ?disabled=${t.length<=1}
            @click=${()=>this.ctx.deleteLayer(e)}
          >${this._trashIcon} Delete</button>
          <button
            class="action-btn"
            title="More actions"
            @click=${i=>{i.stopPropagation(),this._dropdownOpen=!this._dropdownOpen}}
          >&#8943;</button>
        </div>
      </div>
    `}_renderLayerRow(t,e,s){const i=t.id===s,a=e.findIndex(c=>c.id===t.id),o=a===e.length-1,r=a===0,n=this._editingLayerId===t.id;return g`
      <div
        class="layer-row ${i?"active":""} ${this._draggedLayerId===t.id?"dragging":""}"
        data-layer-id=${t.id}
        @click=${()=>this._selectLayer(t.id)}
        @contextmenu=${c=>this._onContextMenu(c,t.id)}
        @pointerdown=${c=>this._onReorderPointerDown(t,c)}
        @pointermove=${c=>this._onReorderPointerMove(c)}
        @pointerup=${c=>this._onReorderPointerUp(c)}
        @pointercancel=${c=>this._onReorderPointerCancel(c)}
      >
        <div
          class="layer-row-main"
        >
          <button
            class="vis-btn ${t.visible?"":"hidden"}"
            title=${t.visible?"Hide layer":"Show layer"}
            @click=${c=>this._toggleVisibility(t,c)}
          >
            ${t.visible?this._eyeOpen:this._eyeClosed}
          </button>

          <canvas class="layer-thumb" width="48" height="36"></canvas>

          ${n?g`<input
                class="layer-name-input"
                aria-label="Layer name"
                .value=${t.name}
                @keydown=${c=>this._onRenameKeyDown(t.id,c)}
                @blur=${c=>this._onRenameBlur(t.id,c)}
                @click=${c=>c.stopPropagation()}
                ${this._autoFocusDirective()}
              />`:g`<span
                class="layer-name"
                @dblclick=${c=>this._startRename(t.id,c)}
              >${t.name}</span>`}

          ${this.ctx.isMobile?g`
            <button
              class="rename-btn"
              title="Rename"
              @click=${c=>{c.stopPropagation(),this._startRename(t.id,c)}}
            >
              <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M11.5 1.5l3 3L5 14H2v-3L11.5 1.5z"/>
              </svg>
            </button>
          `:""}

          <div class="reorder-btns">
            <button
              class="reorder-btn"
              title="Move up"
              ?disabled=${o}
              @click=${c=>this._moveUp(t,c)}
            >&#9650;</button>
            <button
              class="reorder-btn"
              title="Move down"
              ?disabled=${r}
              @click=${c=>this._moveDown(t,c)}
            >&#9660;</button>
          </div>
        </div>

        ${i?g`
            <div class="opacity-row">
              <select class="blend-mode-select"
                aria-label=${`Blend mode of ${t.name}`}
                .value=${t.blendMode}
                @change=${c=>this.ctx.setLayerBlendMode(t.id,c.target.value)}>
                ${Object.entries(ki).map(([c,h])=>g`
                  <option value=${c}>${h}</option>
                `)}
              </select>
            </div>
            <div class="opacity-row">
              <input
                type="range"
                min="0"
                max="100"
                aria-label=${`Opacity of ${t.name}`}
                .value=${String(Math.round(t.opacity*100))}
                @pointerdown=${()=>this._onOpacityPointerDown(t)}
                @input=${c=>this._onOpacityInput(t.id,c)}
                @change=${c=>this._onOpacityChange(t.id,c)}
              />
              <span class="opacity-value">${Math.round(t.opacity*100)}%</span>
            </div>
          `:P}
      </div>
    `}_autoFocusDirective(){return requestAnimationFrame(()=>{const t=this.shadowRoot?.querySelector(".layer-name-input");t&&(t.focus(),t.select())}),P}willUpdate(){this.ctx?.isMobile&&!this._syncingSheet&&(this._syncingSheet=!0,this.ctx.state.layersPanelOpen&&!this._sheetOpen?this.openSheet():!this.ctx.state.layersPanelOpen&&this._sheetOpen&&this.closeSheet(),this._syncingSheet=!1)}updated(t){super.updated(t);const e=this._ctx.value?.state.layers??null,s=this.shadowRoot?.querySelectorAll(".layer-thumb").length??0;e!==this._lastThumbLayers||s!==this._lastThumbRowCount?(this._lastThumbLayers=e,this._lastThumbRowCount=s,this._updateThumbnails()):this._thumbnailScheduler.schedule()}_updateThumbnails(){const t=this._ctx.value?.state.layers??[],e=this.shadowRoot?.querySelectorAll(".layer-thumb");if(!e)return;const s=[...t].reverse();e.forEach((i,a)=>{const o=s[a];if(!o)return;const r=i.getContext("2d");if(r.clearRect(0,0,i.width,i.height),this._drawMiniCheckerboard(r,i.width,i.height),r.globalAlpha=o.opacity,r.drawImage(o.canvas,0,0,i.width,i.height),this._floatDetail&&o.id===this._floatDetail.layerId){const{tempCanvas:n,rect:c,rotation:h}=this._floatDetail,l=o.canvas.width,d=o.canvas.height,f=c.x/l*i.width,u=c.y/d*i.height,p=c.w/l*i.width,_=c.h/d*i.height;if(h){const m=f+p/2,v=u+_/2;r.save(),r.translate(m,v),r.rotate(h),r.drawImage(n,-p/2,-_/2,p,_),r.restore()}else r.drawImage(n,f,u,p,_)}r.globalAlpha=1})}_drawMiniCheckerboard(t,e,s){if(!this._miniCheckerboardPattern){const a=document.createElement("canvas");a.width=8,a.height=8;const o=a.getContext("2d");o.fillStyle="#ffffff",o.fillRect(0,0,8,8),o.fillStyle="#e0e0e0",o.fillRect(4,0,4,4),o.fillRect(0,4,4,4),this._miniCheckerboardPattern=t.createPattern(a,"repeat")}t.fillStyle=this._miniCheckerboardPattern,t.fillRect(0,0,e,s)}};V.styles=bt`
    :host {
      display: flex;
      flex-direction: column;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 0.8125rem;
      color: #ddd;
      user-select: none;
    }

    /* ── Panel (expanded) ─────────────────────── */
    .panel {
      display: flex;
      flex-direction: column;
      flex: 1;
      overflow: hidden;
    }

    .panel.collapsed {
    }

    /* ── Collapsed strip ──────────────────────── */
    .collapsed-strip {
      display: flex;
      flex-direction: column;
      align-items: center;
      width: 32px;
      height: 100%;
      padding-top: 8px;
      gap: 4px;
    }

    .expand-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      font-size: 0.75rem;
      padding: 0;
    }

    .expand-btn:hover {
      background: #444;
      color: #fff;
    }

    .vertical-label {
      writing-mode: vertical-rl;
      text-orientation: mixed;
      color: #888;
      font-size: 0.6875rem;
      letter-spacing: 0.05em;
      margin-top: 6px;
    }

    /* ── Header ───────────────────────────────── */
    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 8px;
      border-bottom: 1px solid #444;
      flex-shrink: 0;
    }

    .header-title {
      font-weight: 600;
      font-size: 0.8125rem;
    }

    .collapse-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 2px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #aaa;
      cursor: pointer;
      font-size: 0.6875rem;
      padding: 2px 6px;
      white-space: nowrap;
    }

    .collapse-btn:hover {
      background: #444;
      color: #fff;
    }

    /* ── Layer list ────────────────────────────── */
    .layer-list {
      flex: 1;
      overflow-y: auto;
      overflow-x: hidden;
      scrollbar-width: thin;
      scrollbar-color: #555 transparent;
    }

    /* ── Layer row ─────────────────────────────── */
    .layer-row {
      position: relative;
      display: flex;
      flex-direction: column;
      padding: 6px 8px;
      min-height: 44px;
      background: #3a3a3a;
      border-bottom: 1px solid #333;
      cursor: pointer;
      transition: background 0.1s ease;
      touch-action: none;
    }

    .layer-row.dragging {
      opacity: 0.4;
    }

    .layer-row.drop-above::before {
      content: '';
      position: absolute;
      top: -1px;
      left: 0;
      right: 0;
      height: 2px;
      background: #5b8cf7;
    }

    .layer-row.drop-below::after {
      content: '';
      position: absolute;
      bottom: -1px;
      left: 0;
      right: 0;
      height: 2px;
      background: #5b8cf7;
    }

    .layer-row:hover {
      background: #424242;
    }

    .layer-row.active {
      background: #3a3a5c;
    }

    .layer-row-main {
      display: flex;
      align-items: center;
      gap: 4px;
      min-height: 24px;
      cursor: grab;
    }

    .layer-row-main:active {
      cursor: grabbing;
    }

    /* ── Visibility button ─────────────────────── */
    .vis-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 22px;
      height: 22px;
      border: none;
      border-radius: 4px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .vis-btn:hover {
      background: #555;
      color: #fff;
    }

    .vis-btn.hidden {
      color: #666;
    }

    .vis-btn svg {
      width: 14px;
      height: 14px;
    }

    /* ── Layer thumbnail ──────────────────────── */
    .layer-thumb {
      width: 48px;
      height: 36px;
      border-radius: 3px;
      border: 1px solid #555;
      flex-shrink: 0;
    }

    /* ── Layer name ─────────────────────────────── */
    .layer-name {
      flex: 1;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 0.75rem;
      line-height: 22px;
    }

    .layer-name-input {
      flex: 1;
      min-width: 0;
      font-size: 0.75rem;
      background: #222;
      border: 1px solid #5b8cf7;
      border-radius: 3px;
      color: #ddd;
      padding: 1px 4px;
      outline: 2px solid transparent;
      font-family: inherit;
    }

    /* ── Reorder buttons ───────────────────────── */
    .reorder-btns {
      display: flex;
      flex-direction: column;
      gap: 0px;
      flex-shrink: 0;
    }

    .reorder-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 16px;
      height: 12px;
      border: none;
      border-radius: 3px;
      background: transparent;
      color: #888;
      cursor: pointer;
      font-size: 0.5rem;
      padding: 0;
      line-height: 1;
    }

    .reorder-btn:hover:not(:disabled) {
      background: #555;
      color: #fff;
    }

    .reorder-btn:disabled {
      opacity: 0.25;
      cursor: default;
    }

    /* ── Opacity slider ────────────────────────── */
    .opacity-row {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-top: 4px;
      padding-left: 26px;
    }

    .opacity-row input[type="range"] {
      flex: 1;
      height: 4px;
      accent-color: #5b8cf7;
      min-width: 0;
    }

    .opacity-value {
      font-size: 0.6875rem;
      color: #aaa;
      min-width: 28px;
      text-align: right;
    }

    /* ── Action bar ────────────────────────────── */
    .action-bar {
      display: flex;
      justify-content: center;
      gap: 6px;
      padding: 6px 8px;
      border-top: 1px solid #444;
      flex-shrink: 0;
    }

    .action-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 4px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      font-size: 0.6875rem;
      padding: 4px 10px;
      font-family: inherit;
      transition: all 0.15s ease;
    }

    .action-btn:hover:not(:disabled) {
      background: #444;
      color: #fff;
    }

    .action-btn:disabled {
      opacity: 0.3;
      cursor: default;
    }

    .action-btn svg {
      width: 14px;
      height: 14px;
    }

    /* ── Context menu ──────────────────────────── */
    .context-menu {
      position: fixed;
      z-index: 300;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 6px;
      padding: 4px 0;
      min-width: 160px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
    }

    .context-menu-item {
      display: block;
      width: 100%;
      padding: 6px 14px;
      border: none;
      background: transparent;
      color: #ddd;
      font-size: 0.8125rem;
      font-family: inherit;
      text-align: left;
      cursor: pointer;
    }

    .context-menu-item:hover:not(:disabled) {
      background: #5b8cf7;
      color: #fff;
    }

    .context-menu-item:disabled {
      color: #666;
      cursor: default;
    }

    /* ── Dropdown menu ─────────────────────────── */
    .action-bar-wrapper {
      position: relative;
    }

    .dropdown-menu {
      position: absolute;
      bottom: 100%;
      right: 0;
      margin-bottom: 4px;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 6px;
      padding: 4px 0;
      min-width: 160px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
      z-index: 300;
    }

    .dropdown-menu button {
      display: block;
      width: 100%;
      padding: 6px 14px;
      border: none;
      background: transparent;
      color: #ddd;
      font-size: 0.8125rem;
      font-family: inherit;
      text-align: left;
      cursor: pointer;
    }

    .dropdown-menu button:hover:not(:disabled) {
      background: #5b8cf7;
      color: #fff;
    }

    .dropdown-menu button:disabled {
      color: #666;
      cursor: default;
    }

    /* ── Mobile bottom sheet ───────────────────── */
    .sheet-backdrop {
      display: none;
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.4);
      z-index: 200;
    }

    .sheet-backdrop.open {
      display: block;
    }

    .sheet {
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      max-height: 90vh;
      background: #2c2c2c;
      border-radius: 16px 16px 0 0;
      z-index: 201;
      display: flex;
      flex-direction: column;
      transition: transform 0.3s ease;
      transform: translateY(100%);
      will-change: transform;
      padding-bottom: env(safe-area-inset-bottom);
    }

    .sheet-handle {
      display: flex;
      justify-content: center;
      padding: 8px 0;
      cursor: grab;
      touch-action: none;
    }

    .sheet-handle-bar {
      width: 36px;
      height: 4px;
      border-radius: 2px;
      background: #666;
    }

    .sheet-content {
      flex: 1;
      overflow-y: auto;
      touch-action: pan-y;
      -webkit-overflow-scrolling: touch;
    }

    .rename-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 28px;
      height: 28px;
      border: none;
      border-radius: 4px;
      background: transparent;
      color: #888;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .rename-btn:hover {
      background: #444;
      color: #ddd;
    }

    .blend-mode-select {
      width: 100%;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 4px;
      padding: 2px 4px;
      font-size: 0.75rem;
      cursor: pointer;
    }
  `;at([M()],V.prototype,"_sheetOpen",2);at([M()],V.prototype,"_sheetY",2);at([M()],V.prototype,"_contextMenuOpen",2);at([M()],V.prototype,"_contextMenuX",2);at([M()],V.prototype,"_contextMenuY",2);at([M()],V.prototype,"_dropdownOpen",2);at([M()],V.prototype,"_editingLayerId",2);at([M()],V.prototype,"_draggedLayerId",2);V=at([yt("layers-panel")],V);var ho=Object.defineProperty,po=Object.getOwnPropertyDescriptor,me=(t,e,s,i)=>{for(var a=i>1?void 0:i?po(e,s):e,o=t.length-1,r;o>=0;o--)(r=t[o])&&(a=(i?r(e,s,a):r(a))||a);return i&&a&&ho(e,s,a),a};let O=class extends F{constructor(){super(...arguments),this._ctx=new gt(this,{context:Et,subscribe:!0}),this._minimapCanvas=document.createElement("canvas"),this._isFullscreen=!1,this._onFullscreenChange=()=>{this._isFullscreen=!!document.fullscreenElement},this._toggleFullscreen=()=>{document.fullscreenElement?document.exitFullscreen():document.documentElement.requestFullscreen()},this._minimapScheduler=Ne(()=>this._renderMinimap(),100),this._onComposited=()=>{this._minimapScheduler.schedule()},this._minimapScale=1,this._minimapOffsetX=0,this._minimapOffsetY=0,this._dragging=!1,this._dragOffsetX=0,this._dragOffsetY=0,this._onMinimapPointerDown=t=>{if(t.button!==0)return;const e=this._getMinimapPoint(t);if(this._isInsideViewportRect(e.x,e.y)){const s=this._getViewportRectInMinimap();this._dragging=!0,this._dragOffsetX=e.x-s.x,this._dragOffsetY=e.y-s.y,this._minimapCanvas.setPointerCapture(t.pointerId)}else{this._panToMinimapPoint(e.x,e.y);const s=this._getViewportRectInMinimap();this._dragging=!0,this._dragOffsetX=s.w/2,this._dragOffsetY=s.h/2,this._minimapCanvas.setPointerCapture(t.pointerId)}},this._onMinimapPointerMove=t=>{if(!this._dragging)return;const e=this._getMinimapPoint(t),s=e.x-this._dragOffsetX,i=e.y-this._dragOffsetY,{zoom:a}=this.ctx,o=this._minimapScale,r=(s-this._minimapOffsetX)/o,n=(i-this._minimapOffsetY)/o,c=-r*a,h=-n*a;this.dispatchEvent(new CustomEvent("navigator-pan",{bubbles:!0,composed:!0,detail:{panX:c,panY:h}}))},this._onMinimapPointerUp=t=>{this._dragging&&(this._dragging=!1,this._minimapCanvas.releasePointerCapture(t.pointerId))},this._onSliderInput=t=>{const e=parseInt(t.target.value,10);this._dispatchZoom(this._sliderToZoom(e))},this._onZoomIn=()=>{this._dispatchZoom(this.ctx.zoom*O.ZOOM_STEP)},this._onZoomOut=()=>{this._dispatchZoom(this.ctx.zoom/O.ZOOM_STEP)},this._editingZoom=!1,this._zoomInputValue="",this._onZoomInputFocus=t=>{this._editingZoom=!0,this._zoomInputValue=Math.round(this.ctx.zoom*100).toString();const e=t.target;requestAnimationFrame(()=>e.select())},this._onZoomInputBlur=()=>{this._commitZoomInput(),this._editingZoom=!1},this._onZoomInputKeydown=t=>{t.key==="Enter"?(this._commitZoomInput(),this._editingZoom=!1,t.target.blur()):t.key==="Escape"&&(this._editingZoom=!1,t.target.blur()),t.stopPropagation()},this._onZoomInputChange=t=>{this._zoomInputValue=t.target.value}}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),this.getRootNode().addEventListener("composited",this._onComposited),document.addEventListener("fullscreenchange",this._onFullscreenChange)}disconnectedCallback(){super.disconnectedCallback(),this.getRootNode().removeEventListener("composited",this._onComposited),this._minimapScheduler.cancel(),document.removeEventListener("fullscreenchange",this._onFullscreenChange)}_renderMinimap(){if(!this._ctx.value)return;const{state:t}=this.ctx,{layers:e,documentWidth:s,documentHeight:i}=t,a=this._minimapCanvas,o=this.shadowRoot?.querySelector(".minimap-container");if(!o)return;const r=o.clientWidth-12;if(r<=0)return;const n=150,c=s/i;let h=r,l=h/c;l>n&&(l=n,h=l*c);const d=window.devicePixelRatio||1,f=Math.round(h*d),u=Math.round(l*d);(a.width!==f||a.height!==u)&&(a.width=f,a.height=u),a.style.width=`${h}px`,a.style.height=`${l}px`;const p=a.getContext("2d");p.setTransform(d,0,0,d,0,0);const _=Math.min(h/s,l/i);this._minimapScale=_;const m=s*_,v=i*_;this._minimapOffsetX=(h-m)/2,this._minimapOffsetY=(l-v)/2,p.fillStyle="#3a3a3a",p.fillRect(0,0,h,l),p.fillStyle="#ffffff",p.fillRect(this._minimapOffsetX,this._minimapOffsetY,m,v),p.save(),p.translate(this._minimapOffsetX,this._minimapOffsetY);const b=e.some(y=>y.visible&&y.blendMode!=="normal");for(const y of e)y.visible&&(p.globalAlpha=y.opacity,b&&(p.globalCompositeOperation=Yt(y.blendMode)),p.drawImage(y.canvas,0,0,m,v));p.globalAlpha=1,p.globalCompositeOperation="source-over",p.restore(),this._drawViewportRect(p,_,h,l)}_drawViewportRect(t,e,s,i){const{zoom:a,panX:o,panY:r,viewportWidth:n,viewportHeight:c}=this.ctx,h=this._minimapOffsetX+-o/a*e,l=this._minimapOffsetY+-r/a*e,d=n/a*e,f=c/a*e,u=Math.max(0,h),p=Math.max(0,l),_=Math.min(s-u,d-(u-h)),m=Math.min(i-p,f-(p-l));_<=0||m<=0||(t.fillStyle="rgba(255, 68, 68, 0.1)",t.fillRect(u,p,_,m),t.strokeStyle="#ff4444",t.lineWidth=1.5,t.strokeRect(u,p,_,m))}_getMinimapPoint(t){const e=this._minimapCanvas.getBoundingClientRect();return{x:t.clientX-e.left,y:t.clientY-e.top}}_getViewportRectInMinimap(){const{zoom:t,panX:e,panY:s,viewportWidth:i,viewportHeight:a}=this.ctx,o=this._minimapScale;return{x:this._minimapOffsetX+-e/t*o,y:this._minimapOffsetY+-s/t*o,w:i/t*o,h:a/t*o}}_isInsideViewportRect(t,e){const s=this._getViewportRectInMinimap();return t>=s.x&&t<=s.x+s.w&&e>=s.y&&e<=s.y+s.h}_panToMinimapPoint(t,e){const{zoom:s,viewportWidth:i,viewportHeight:a}=this.ctx,o=this._minimapScale,r=(t-this._minimapOffsetX)/o,n=(e-this._minimapOffsetY)/o,c=i/2-r*s,h=a/2-n*s;this.dispatchEvent(new CustomEvent("navigator-pan",{bubbles:!0,composed:!0,detail:{panX:c,panY:h}}))}_zoomToSlider(t){const{MIN_ZOOM:e,MAX_ZOOM:s,SLIDER_MAX:i}=O,a=Math.log(t/e)/Math.log(s/e);return Math.round(a*i)}_sliderToZoom(t){const{MIN_ZOOM:e,MAX_ZOOM:s,SLIDER_MAX:i}=O,a=t/i;return e*Math.pow(s/e,a)}_dispatchZoom(t){const e=Math.min(O.MAX_ZOOM,Math.max(O.MIN_ZOOM,t));this.dispatchEvent(new CustomEvent("navigator-zoom",{bubbles:!0,composed:!0,detail:{zoom:e}}))}_commitZoomInput(){const t=this._zoomInputValue.replace("%","").trim(),e=parseFloat(t);if(isNaN(e)||e<=0)return;const s=e/100;this._dispatchZoom(s)}render(){if(!this._ctx.value)return g``;if(!this.ctx.state.layersPanelOpen)return g``;if(this.ctx.isMobile)return g``;const t=this.ctx.zoom,e=this._zoomToSlider(t),s=Math.round(t*100),i=this._editingZoom?this._zoomInputValue:`${s}%`;return g`
      <div class="section">
        <div class="header">
          <span class="header-title">Navigator</span>
        </div>
        <div class="minimap-container"
          @pointerdown=${this._onMinimapPointerDown}
          @pointermove=${this._onMinimapPointerMove}
          @pointerup=${this._onMinimapPointerUp}
        >
          ${this._minimapCanvas}
        </div>
        <div class="zoom-controls">
          <button class="zoom-btn" title="Zoom out" @click=${this._onZoomOut}>&minus;</button>
          <input
            type="range"
            class="zoom-slider"
            aria-label="Zoom"
            min="0"
            max="${O.SLIDER_MAX}"
            step="1"
            .value=${String(e)}
            @input=${this._onSliderInput}
          />
          <button class="zoom-btn" title="Zoom in" @click=${this._onZoomIn}>+</button>
          <input
            type="text"
            class="zoom-input"
            aria-label="Zoom percentage"
            .value=${i}
            @focus=${this._onZoomInputFocus}
            @blur=${this._onZoomInputBlur}
            @keydown=${this._onZoomInputKeydown}
            @input=${this._onZoomInputChange}
          />
          <button
            class="fullscreen-btn"
            title=${this._isFullscreen?"Exit fullscreen":"Fullscreen"}
            @click=${this._toggleFullscreen}
          >
            ${this._isFullscreen?g`<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M5 2v3H2M14 5h-3V2M11 14v-3h3M2 11h3v3"/></svg>`:g`<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4"/></svg>`}
          </button>
        </div>
      </div>
    `}};O.MIN_ZOOM=.1;O.MAX_ZOOM=10;O.ZOOM_STEP=1.1;O.SLIDER_MAX=1e3;O.styles=bt`
    :host {
      display: block;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 0.8125rem;
      color: #ddd;
      user-select: none;
    }

    .section {
      border-bottom: 1px solid #444;
    }

    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 8px;
      border-bottom: 1px solid #444;
    }

    .header-title {
      font-weight: 500;
      font-size: 0.75rem;
      color: #ccc;
    }

    .minimap-container {
      padding: 6px;
    }

    canvas {
      display: block;
      width: 100%;
      border-radius: 3px;
      background: #3a3a3a;
      cursor: crosshair;
    }

    .zoom-controls {
      display: flex;
      align-items: center;
      gap: 4px;
      padding: 4px 6px 6px;
    }

    .zoom-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      border: 1px solid #555;
      border-radius: 4px;
      background: #444;
      color: #ccc;
      cursor: pointer;
      font-size: 14px;
      padding: 0;
      line-height: 1;
      flex-shrink: 0;
    }

    .zoom-btn:hover {
      background: #555;
      color: #fff;
    }

    .zoom-slider {
      flex: 1;
      height: 4px;
      -webkit-appearance: none;
      appearance: none;
      background: #555;
      border-radius: 2px;
      outline: 2px solid transparent;
      min-width: 0;
    }

    .zoom-slider::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 12px;
      height: 12px;
      border-radius: 50%;
      background: #bbb;
      border: 1px solid #888;
      cursor: grab;
    }

    .zoom-slider::-webkit-slider-thumb:active {
      cursor: grabbing;
    }

    .zoom-input {
      width: 38px;
      background: #333;
      border: 1px solid #555;
      border-radius: 3px;
      padding: 1px 3px;
      text-align: center;
      font-size: 0.6875rem;
      color: #ddd;
      font-family: inherit;
      flex-shrink: 0;
    }

    .zoom-input:focus {
      outline: 1px solid #007bff;
      border-color: #007bff;
    }

    .fullscreen-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      border: 1px solid #555;
      border-radius: 4px;
      background: #444;
      color: #ccc;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .fullscreen-btn:hover {
      background: #555;
      color: #fff;
    }
  `;me([M()],O.prototype,"_isFullscreen",2);me([M()],O.prototype,"_editingZoom",2);me([M()],O.prototype,"_zoomInputValue",2);O=me([yt("navigator-panel")],O);var uo=Object.defineProperty,fo=Object.getOwnPropertyDescriptor,z=(t,e,s,i)=>{for(var a=i>1?void 0:i?fo(e,s):e,o=t.length-1,r;o>=0;o--)(r=t[o])&&(a=(i?r(e,s,a):r(a))||a);return i&&a&&uo(e,s,a),a};const _o=768,mo=800;function go(t,e){return e?t<=mo:t<_o}let D=class extends F{constructor(){super(),this._layerCounter=0,this._canUndo=!1,this._canRedo=!1,this._saving=!1,this._viewportZoom=1,this._viewportPanX=0,this._viewportPanY=0,this._viewportWidth=800,this._viewportHeight=600,this._currentProject=null,this._projectList=[],this._isMobile=!1,this._mobileObserver=null,this.embedded=!1,this._storageState="loading",this._ownsBackend=!1,this._autosave=!0,this._ready=new Promise((e,s)=>{this._resolveReady=e,this._rejectReady=s}),this._savedDocument={top:null,trimmed:0,generation:0},this._documentGeneration=0,this._exportedDocument=null,this._exportMarks=new WeakMap,this._lastReportedModified=!1,this._documentReplacement=Promise.resolve(),this._dirty=!1,this._saveTimer=null,this._saveInProgress=!1,this._savePromise=null,this._saveRequested=!1,this._forceFlushNextSave=!1,this._dirtyVersion=0,this._contentVersion=0,this._savedContentVersion=-1,this._unsavedWork=!1,this._savedHistory=new Map,this._nextHistoryRecordIndex=0,this._historyNeedsRewrite=!1,this._savedLayerBlobs=new Map,this._trackedProjectId=null,this._trackingGeneration=0,this._projectLoads=0,this._desktopLayersPanelOpen=null,this._onBeforeUnload=e=>{this.embedded||(this.canvas?.clearSelection(),this._dirty&&(this._flushPendingSave(),e.preventDefault()))},this._onVisibilityChange=()=>{document.hidden&&(this.canvas?.clearSelection(),this._dirty&&this._flushPendingSave())},this._onKeyDown=e=>{if((e.ctrlKey||e.metaKey)&&!e.shiftKey&&!e.altKey&&e.key.toLowerCase()==="s"&&this.embedded){e.preventDefault(),e.repeat||this._requestSave();return}if(this._isTextEntryTarget(e))return;if(e.key==="Escape"&&this.canvas?.hasExternalFloat){e.preventDefault(),this.canvas.cancelExternalFloat();return}if(e.key==="Escape"&&this.canvas?.isTransformActive()){e.preventDefault(),this.canvas.cancelTransform();return}if(e.key==="Enter"&&this.canvas?.isTransformActive()){e.preventDefault(),this.canvas.commitTransform();return}const s=e.ctrlKey||e.metaKey,i=e.key.toLowerCase();if(s&&i==="t"){e.preventDefault(),this.canvas?.enterTransformMode();return}if(s&&i==="z"&&!e.shiftKey)e.preventDefault(),this.canvas?.undo();else if(s&&(i==="y"||i==="z"&&e.shiftKey))e.preventDefault(),this.canvas?.redo();else if(s&&i==="c")e.preventDefault(),this.canvas?.copySelection();else if(s&&i==="x")e.preventDefault(),this.canvas?.cutSelection();else if(s&&i==="v")e.preventDefault(),this.canvas?.paste();else if(s&&i==="d")e.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty("setting")),this.canvas?.duplicateInPlace();else if((e.key==="Delete"||e.key==="Backspace")&&(this._state.activeTool==="select"||this._state.activeTool==="stamp"))e.preventDefault(),this.canvas?.deleteSelection();else if(e.key==="Enter"&&this._state.activeTool==="crop"&&this.canvas?.hasCropRect)e.preventDefault(),this.canvas.commitCrop();else if(e.key==="Escape")this._state.activeTool==="crop"&&this.canvas?.hasCropRect?this.canvas.cancelCrop():this.canvas?.hasExternalFloat?this.canvas.cancelExternalFloat():this.canvas?.clearSelection();else if(s&&i==="a"&&e.shiftKey)e.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty("setting")),this.canvas?.selectAllCanvas();else if(s&&i==="a"&&!e.shiftKey)e.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty("setting")),this.canvas?.selectAll();else if(e.key==="0"&&s)e.preventDefault(),this.canvas?.zoomToFit();else if(s&&(e.key==="="||e.key==="+"))e.preventDefault(),this.canvas?.zoomIn();else if(s&&e.key==="-")e.preventDefault(),this.canvas?.zoomOut();else if(!s&&!e.altKey&&(i==="["||i==="]")){if(e.preventDefault(),this._state.activeTool==="stamp"){const n=this._state.stampSize,c=i==="]"?Math.max(n+1,Math.round(n*1.1)):Math.min(n-1,Math.round(n/1.1));this._updateStampSize(c);return}const a=this._state.brush.size,o=150,r=1;if(i==="]"){const n=Math.min(o,Math.max(a+1,Math.round(a*1.1)));this._updateBrush({size:n})}else{const n=Math.max(r,Math.min(a-1,Math.round(a/1.1)));this._updateBrush({size:n})}}else if(!s&&!e.altKey&&(e.key==="{"||e.key==="}")){e.preventDefault();const a=this._state.brush.hardness;e.key==="}"?this._updateBrush({hardness:Math.round(Math.min(1,a+.1)*10)/10}):this._updateBrush({hardness:Math.round(Math.max(0,a-.1)*10)/10})}else if(!s&&!e.altKey&&!e.shiftKey&&i.length===1){const a=aa(i);a&&a!==this._state.activeTool&&(e.preventDefault(),this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:a},this._markDirty("setting"))}},this._ready.catch(()=>{});const t=this._createLayer(800,600);this._state={activeTool:"pencil",strokeColor:"#000000",fillColor:"#ff0000",useFill:!1,brush:ie(),activePreset:"round",isPresetModified:!1,stampImage:null,activeStampId:null,stampSize:Ie,layers:[t],activeLayerId:t.id,layersPanelOpen:!0,documentWidth:800,documentHeight:600,cropAspectRatio:"free",fontFamily:"sans-serif",fontSize:24,fontBold:!1,fontItalic:!1,eyedropperSampleAll:!0,childMode:!1},this._provider=new ye(this,{context:Et,initialValue:this._buildContextValue()})}_createLayer(t,e){this._layerCounter++;const s=document.createElement("canvas");return s.width=t,s.height=e,{id:it(),name:`Layer ${this._layerCounter}`,visible:!0,opacity:1,blendMode:"normal",canvas:s}}_snapshotLayer(t){const e=t.canvas.getContext("2d");return{id:t.id,name:t.name,visible:t.visible,opacity:t.opacity,blendMode:t.blendMode,imageData:e.getImageData(0,0,t.canvas.width,t.canvas.height)}}_snapshotAllLayers(){return this._state.layers.map(t=>this._snapshotLayer(t))}_compositeLayers(t,e="#ffffff"){const s=this._state.documentWidth,i=this._state.documentHeight,a=document.createElement("canvas");a.width=s,a.height=i;const o=a.getContext("2d");e&&(o.fillStyle=e,o.fillRect(0,0,s,i));for(const r of t)o.globalAlpha=r.opacity,o.globalCompositeOperation=Yt(r.blendMode),o.drawImage(r.canvas,0,0),o.globalCompositeOperation="source-over";return o.globalAlpha=1,a}_flushPendingSave(){this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),this._save(!0)}async _flushPendingSaveAndWait(){this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),await this._save(!0)}_markDirty(t="work"){t!=="viewport"&&this._contentVersion++,this._autosave&&(t==="work"&&(this._unsavedWork=!0),this._dirty=!0,this._dirtyVersion++,this._saveRequested=!0,this._saveTimer&&clearTimeout(this._saveTimer),this._saveTimer=setTimeout(()=>{this._save()},500))}_updateBrush(t){this._state={...this._state,brush:{...this._state.brush,...t},isPresetModified:!0},this._markDirty("setting")}_updateStampSize(t){const e=ns(t,this._state.stampSize);e!==this._state.stampSize&&(this._state={...this._state,stampSize:e},this._markDirty("setting"))}_planHistorySave(t,e){const s=this._savedHistory;let i=this._historyNeedsRewrite||this._trackedProjectId!==t||!this._backend?.history.updateEntries;if(!i){let o=-1,r=!1;for(const n of e){const c=s.get(n);if(!c)r=!0;else if(r||c.index<=o){i=!0;break}else o=c.index}}if(i)return{rewrite:i,remove:[...s],add:e,firstIndex:0};const a=new Set(e);return{rewrite:i,remove:[...s].filter(([o])=>!a.has(o)),add:e.filter(o=>!s.has(o)),firstIndex:this._nextHistoryRecordIndex}}_recordSavedHistory(t,e,s){this._trackedProjectId=t,e.rewrite&&this._savedHistory.clear();for(const[i]of e.remove)this._savedHistory.delete(i);e.add.forEach((i,a)=>{const o=new Set;It(s[a].entry,o),this._savedHistory.set(i,{index:s[a].index,blobRefs:[...o]})}),this._nextHistoryRecordIndex=e.firstIndex+e.add.length,this._historyNeedsRewrite=!1}_trackLoadedProject(t,e,s,i=new Map){this._trackedProjectId=t,this._trackingGeneration++,this._savedHistory=new Map(e.map((a,o)=>{const r=new Set;return It(s[o].entry,r),[a,{index:s[o].index,blobRefs:[...r]}]})),this._nextHistoryRecordIndex=s.reduce((a,o)=>Math.max(a,o.index+1),0),this._historyNeedsRewrite=!1,this._savedLayerBlobs=i,this._savedContentVersion=-1,this._unsavedWork=!1}async _enterProject(t,e){this._projectLoads++;try{this._currentProject=t,await e()}finally{this._projectLoads--}}_renderThumbnail(t){const e=Math.min(1,D.THUMBNAIL_SIZE/Math.max(t.width,t.height,1)),s=document.createElement("canvas");return s.width=Math.max(1,Math.round(t.width*e)),s.height=Math.max(1,Math.round(t.height*e)),s.getContext("2d").drawImage(t,0,0,s.width,s.height),s}async _save(t=!1){if(this._savePromise)return t&&(this._forceFlushNextSave=!0),this._dirty&&(this._saveRequested=!0),this._savePromise;if(!(!this._currentProject||!this._dirty||this._projectLoads>0)&&this._backend){this._savePromise=(async()=>{this._saveInProgress=!0;let e=t;try{for(;this._currentProject&&this._dirty&&this._projectLoads===0;){const s=this._currentProject.id,i=this._dirtyVersion,a=this._contentVersion,o=Date.now(),r=this._forceFlushNextSave;this._forceFlushNextSave=!1,this._saveRequested=!1;const n=e||r;this._unsavedWork&&(this._unsavedWork=!1,this._saving=!0);const c={activeTool:this._state.activeTool,strokeColor:this._state.strokeColor,fillColor:this._state.fillColor,useFill:this._state.useFill,brushSize:this._state.brush.size,stampSize:this._state.stampSize,opacity:this._state.brush.opacity,flow:this._state.brush.flow,hardness:this._state.brush.hardness,spacing:this._state.brush.spacing,pressureSize:this._state.brush.pressureSize,pressureOpacity:this._state.brush.pressureOpacity,pressureCurve:this._state.brush.pressureCurve,tip:{...this._state.brush.tip},ink:{...this._state.brush.ink},activePreset:this._state.activePreset,isPresetModified:this._state.isPresetModified,cropAspectRatio:this._state.cropAspectRatio,fontFamily:this._state.fontFamily,fontSize:this._state.fontSize,fontBold:this._state.fontBold,fontItalic:this._state.fontItalic,eyedropperSampleAll:this._state.eyedropperSampleAll,childMode:this._state.childMode},h=this._state.documentWidth,l=this._state.documentHeight,d=this._state.activeLayerId,f=this._desktopLayersPanelOpen??this._state.layersPanelOpen,u=this.canvas?.getFloatSnapshot()??null,p=!u&&a===this._savedContentVersion&&this._trackedProjectId===s&&this._state.layers.every(w=>this._savedLayerBlobs.has(w.id)),_=this._state.layers.map(w=>{const L={id:w.id,name:w.name,visible:w.visible,opacity:w.opacity,blendMode:w.blendMode};if(p)return{...L,imageData:null};const Ve=w.canvas.getContext("2d").getImageData(0,0,w.canvas.width,w.canvas.height);if(u&&w.id===u.layerId){const At=document.createElement("canvas");At.width=w.canvas.width,At.height=w.canvas.height;const ve=At.getContext("2d");return ve.putImageData(Ve,0,0),ve.drawImage(u.tempCanvas,u.x,u.y),{...L,imageData:ve.getImageData(0,0,At.width,At.height)}}return{...L,imageData:Ve}}),m=_.map(w=>w.imageData?os(w.imageData):this._savedLayerBlobs.get(w.id).hash),v=this.canvas?.getViewport()??{zoom:1,panX:0,panY:0},b=this.canvas?.getViewportSize()??null,y=this.canvas?.getHistory()??[],x=this.canvas?.getHistoryIndex()??-1,C=this._trackingGeneration,k=this._planHistorySave(s,y),T=k.rewrite,S=this._backend.blobs,[E,H,B]=await Promise.all([this._backend.state.get(s),this._backend.projects.get(s),T?this._backend.history.getEntries(s):Promise.resolve([])]);if(!H)break;const Lt=E?.layers.map(w=>w.imageBlobRef)??[],q=H.thumbnailRef??null,Z=[],Fe={get:w=>S.get(w),delete:w=>S.delete(w),deleteMany:w=>S.deleteMany(w),gc:S.gc?w=>S.gc(w):void 0,async put(w){const L=await S.put(w);return Z.push(L),L}};let Kt,wt;try{Kt=await Promise.all(_.map((w,L)=>{const U=this._savedLayerBlobs.get(w.id);return U&&U.hash===m[L]&&Lt.includes(U.blobRef)?{id:w.id,name:w.name,visible:w.visible,opacity:w.opacity,blendMode:w.blendMode,imageBlobRef:U.blobRef}:Qi(w,w.imageData,Fe)})),wt=await Promise.all(k.add.map(async(w,L)=>({projectId:s,index:k.firstIndex+L,entry:await ea(w,Fe)})))}catch(w){throw Z.length>0&&S.deleteMany(Z).catch(()=>{}),w}const Gs={projectId:s,toolSettings:c,canvasWidth:h,canvasHeight:l,layers:Kt,activeLayerId:d,layersPanelOpen:f,historyIndex:x,zoom:v.zoom,panX:v.panX,panY:v.panY,viewportWidth:b?.width,viewportHeight:b?.height};let ge=null;if(this.canvas?.mainCanvas)try{ge=await Ye(this._renderThumbnail(this.canvas.mainCanvas))}catch{}try{await this._backend.state.save(Gs),T?await this._backend.history.replaceAll(s,wt):(k.remove.length>0||wt.length>0)&&await this._backend.history.updateEntries(s,k.remove.map(([,w])=>w.index),wt)}catch(w){throw E&&this._backend.state.save(E).catch(L=>{console.error("Failed to rollback state after save failure:",L)}),S.deleteMany(Z).catch(()=>{}),w}this._currentProject?.id===s&&this._trackingGeneration===C&&(this._recordSavedHistory(s,k,wt),this._savedLayerBlobs=new Map(_.map((w,L)=>[w.id,{hash:m[L],blobRef:Kt[L].imageBlobRef}])),this._savedContentVersion=a);let xt=q;try{ge?(xt=await S.put(ge),await this._backend.projects.update(s,{thumbnailRef:xt})):await this._backend.projects.update(s,{})}catch{xt!==q&&xt&&S.delete(xt).catch(()=>{})}const Js=new Set(Kt.map(w=>w.imageBlobRef)),zt=Lt.filter(w=>!Js.has(w));if(q&&q!==xt&&zt.push(q),!T)for(const[,w]of k.remove)zt.push(...w.blobRefs);if(T&&B.length>0){const w=new Set;for(const U of B)It(U.entry,w);const L=new Set;for(const U of wt)It(U.entry,L);for(const U of w)L.has(U)||zt.push(U)}if(zt.length>0&&S.deleteMany(zt).catch(()=>{}),this._currentProject?.id===s&&this._dirtyVersion===i&&(this._dirty=!1),this._projectList=await this._backend.projects.list(),!n){const w=Date.now()-o;w<1500&&await new Promise(L=>setTimeout(L,1500-w))}if(!this._saveRequested||!this._dirty)break;e=!1}}catch(s){s instanceof As?console.error("Storage quota exceeded. Consider deleting old projects to free space."):console.error("Save failed:",s)}finally{this._saving=!1,this._saveInProgress=!1}})();try{await this._savePromise}finally{this._savePromise=null}}}_isTextEntryTarget(t){for(const e of t.composedPath())if(e instanceof HTMLElement){if(e.isContentEditable||e instanceof HTMLTextAreaElement)return!0;if(e instanceof HTMLInputElement)return!D.NON_TEXT_INPUT_TYPES.has(e.type);if(e instanceof HTMLDialogElement&&e.open)return!0}return!1}_onCommitOpacity(t){const{layerId:e,before:s,after:i}=t.detail;this.canvas?.pushLayerOperation({type:"opacity",layerId:e,before:s,after:i}),this._markDirty()}_onCropCommit(t){const{width:e,height:s}=t.detail;this._applyDocumentDimensions(e,s),this._state={...this._state,layers:[...this._state.layers]},this._markDirty()}async _resetToFreshProject(t=800,e=600,s="#ffffff"){this.canvas?.clearSelection(),this._layerCounter=0;const i=t,a=e,o=this._createLayer(i,a);if(this._state={activeTool:"pencil",strokeColor:"#000000",fillColor:"#ff0000",useFill:!1,brush:ie(),activePreset:"round",isPresetModified:!1,stampImage:null,activeStampId:null,stampSize:Ie,layers:[o],activeLayerId:o.id,layersPanelOpen:!this._isMobile,documentWidth:i,documentHeight:a,cropAspectRatio:"free",fontFamily:"sans-serif",fontSize:24,fontBold:!1,fontItalic:!1,eyedropperSampleAll:!0,childMode:!1},await this.updateComplete,this.canvas?.setHistory([],-1),s){const r=o.canvas.getContext("2d");r.fillStyle=s,r.fillRect(0,0,o.canvas.width,o.canvas.height)}this.canvas?.composite(),this._dirty=!1,this._trackLoadedProject(this._currentProject?.id??null,[],[]),this._historyNeedsRewrite=!0,this._isMobile&&(this._desktopLayersPanelOpen=!0)}async _loadProject(t){try{this.canvas?.clearSelection();const e=await this._backend.state.get(t);if(!e){await this._resetToFreshProject();return}const s=16384;if(e.canvasWidth<=0||e.canvasWidth>s||e.canvasHeight<=0||e.canvasHeight>s){console.error("Invalid canvas dimensions in saved state:",e.canvasWidth,e.canvasHeight),await this._resetToFreshProject();return}const i=this._backend.blobs,a=await Promise.all(e.layers.map(u=>ta(u,e.canvasWidth,e.canvasHeight,i)));if(a.length===0){await this._resetToFreshProject();return}const o=await this._backend.history.getEntries(t),r=await Promise.all(o.map(u=>sa(u.entry,i))),n=new Map(a.map((u,p)=>{const _=u.canvas.getContext("2d").getImageData(0,0,u.canvas.width,u.canvas.height);return[u.id,{hash:os(_),blobRef:e.layers[p].imageBlobRef}]})),c=a.reduce((u,p)=>{const _=p.name.match(/^Layer (\d+)$/);return _?Math.max(u,parseInt(_[1])):u},0);this._layerCounter=c;const h=a.some(u=>u.id===e.activeLayerId)?e.activeLayerId:a[0].id,l=e.toolSettings,d=ie(),f={size:l.brushSize??d.size,opacity:l.opacity??d.opacity,flow:l.flow??d.flow,hardness:l.hardness??d.hardness,spacing:l.spacing??d.spacing,pressureSize:l.pressureSize??d.pressureSize,pressureOpacity:l.pressureOpacity??d.pressureOpacity,pressureCurve:l.pressureCurve??d.pressureCurve,tip:{...d.tip,...l.tip??{}},ink:{...d.ink,...l.ink??{}}};if(this._state={activeTool:l.activeTool==="marker"?"pencil":l.activeTool,strokeColor:l.strokeColor,fillColor:l.fillColor,useFill:l.useFill,brush:f,activePreset:l.activePreset??"round",isPresetModified:l.isPresetModified??!1,stampImage:null,activeStampId:null,stampSize:ns(l.stampSize),layers:a,activeLayerId:h,layersPanelOpen:e.layersPanelOpen&&!this._isMobile,documentWidth:e.canvasWidth,documentHeight:e.canvasHeight,cropAspectRatio:l.cropAspectRatio??"free",fontFamily:l.fontFamily??"sans-serif",fontSize:l.fontSize??24,fontBold:l.fontBold??!1,fontItalic:l.fontItalic??!1,eyedropperSampleAll:l.eyedropperSampleAll??!0,childMode:l.childMode??!1},this._isMobile&&(this._desktopLayersPanelOpen=e.layersPanelOpen),await this.updateComplete,this.canvas?.setHistory(r,e.historyIndex??r.length-1),this._dirty=!1,this._trackLoadedProject(t,r,o,n),e.zoom!=null&&e.panX!=null&&e.panY!=null){const u=e.viewportWidth!=null&&e.viewportHeight!=null?{width:e.viewportWidth,height:e.viewportHeight}:void 0;this.canvas?.restoreViewport(e.zoom,e.panX,e.panY,u)}else this.canvas?.resetView()}catch(e){console.error("Failed to load project:",e),await this._resetToFreshProject()}}_applyDocumentDimensions(t,e){this._state={...this._state,documentWidth:t,documentHeight:e}}_buildContextValue(){return{state:this._state,setTool:t=>{this._state.activeTool!==t&&(this.canvas?.isTransformActive()&&this.canvas.commitTransform(),this.canvas?.cancelCrop(),this.canvas?.clearSelection()),this._state={...this._state,activeTool:t},this._markDirty("setting")},setStrokeColor:t=>{this._state={...this._state,strokeColor:t},this._markDirty("setting")},setFillColor:t=>{this._state={...this._state,fillColor:t},this._markDirty("setting")},setUseFill:t=>{this._state={...this._state,useFill:t},this._markDirty("setting")},setBrushSize:t=>{const e=Number.isNaN(t)?this._state.brush.size:t;this._updateBrush({size:Math.max(1,Math.min(150,e))})},setStampSize:t=>{this._updateStampSize(t)},setStampImage:(t,e=null)=>{this._state={...this._state,stampImage:t,activeStampId:t?e:null},this._markDirty("setting")},undo:()=>this.canvas?.undo(),redo:()=>this.canvas?.redo(),clearCanvas:()=>this.canvas?.clearCanvas(),saveCanvas:()=>this._requestSave(),embedded:this.embedded,addLayer:t=>{this.canvas?.clearSelection();const e=this._createLayer(this._state.documentWidth,this._state.documentHeight);t&&(e.name=t,this._layerCounter--);const i=this._state.layers.findIndex(o=>o.id===this._state.activeLayerId)+1,a=[...this._state.layers];return a.splice(i,0,e),this._state={...this._state,layers:a,activeLayerId:e.id},this.canvas?.pushLayerOperation({type:"add-layer",layer:this._snapshotLayer(e),index:i}),this._markDirty(),e.id},deleteLayer:t=>{if(this._state.layers.length<=1)return;const e=this._state.layers.findIndex(r=>r.id===t);if(e===-1)return;t===this._state.activeLayerId&&this.canvas?.clearSelection();const s=this._state.layers[e],i=this._snapshotLayer(s),a=this._state.layers.filter(r=>r.id!==t),o=this._state.activeLayerId===t?a[Math.min(e,a.length-1)].id:this._state.activeLayerId;this._state={...this._state,layers:a,activeLayerId:o},this.canvas?.pushLayerOperation({type:"delete-layer",layer:i,index:e}),this._markDirty()},setActiveLayer:t=>{this._state.layers.some(e=>e.id===t)&&t!==this._state.activeLayerId&&(this.canvas?.clearSelection(),this._state={...this._state,activeLayerId:t},this._markDirty("setting"))},setLayerVisibility:(t,e)=>{const s=this._state.layers.find(o=>o.id===t);if(!s||s.visible===e)return;const i=s.visible,a=this._state.layers.map(o=>o.id===t?{...o,visible:e}:o);this._state={...this._state,layers:a},this.canvas?.pushLayerOperation({type:"visibility",layerId:t,before:i,after:e}),this._markDirty()},setLayerOpacity:(t,e)=>{if(!this._state.layers.find(r=>r.id===t))return;const i=Number.isFinite(e)?e:1,a=Math.max(0,Math.min(1,i)),o=this._state.layers.map(r=>r.id===t?{...r,opacity:a}:r);this._state={...this._state,layers:o},this._markDirty()},reorderLayer:(t,e)=>{const s=this._state.layers.findIndex(r=>r.id===t);if(s===-1||s===e)return;const i=[...this._state.layers],[a]=i.splice(s,1),o=e<0?Math.max(i.length+e,0):Math.min(e,i.length);i.splice(o,0,a),this._state={...this._state,layers:i},this.canvas?.pushLayerOperation({type:"reorder",fromIndex:s,toIndex:o}),this._markDirty()},renameLayer:(t,e)=>{const s=this._state.layers.find(o=>o.id===t);if(!s||s.name===e)return;const i=s.name,a=this._state.layers.map(o=>o.id===t?{...o,name:e}:o);this._state={...this._state,layers:a},this.canvas?.pushLayerOperation({type:"rename",layerId:t,before:i,after:e}),this._markDirty()},setLayerBlendMode:(t,e)=>{const s=this._state.layers.find(o=>o.id===t);if(!s||s.blendMode===e)return;const i=s.blendMode,a=this._state.layers.map(o=>o.id===t?{...o,blendMode:e}:o);this._state={...this._state,layers:a},this.canvas?.pushLayerOperation({type:"blend-mode",layerId:t,before:i,after:e}),this._markDirty()},mergeLayerDown:t=>{const e=this._state.layers,s=e.findIndex(l=>l.id===t);if(s<=0)return;this.canvas?.clearSelection();const i=this._snapshotAllLayers(),a=this._state.activeLayerId,o=e[s-1],r=e[s],n=this._compositeLayers([o,r],null),c=e.filter(l=>l.id!==r.id).map(l=>l.id===o.id?{...l,canvas:n,opacity:1,blendMode:"normal"}:l);this._state={...this._state,layers:c,activeLayerId:o.id};const h=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:i,afterLayers:h,previousActiveLayerId:a,afterActiveLayerId:o.id}),this._markDirty()},mergeVisibleLayers:()=>{const t=this._state.layers,e=t.filter(d=>d.visible);if(e.length<2)return;this.canvas?.clearSelection();const s=this._snapshotAllLayers(),i=this._state.activeLayerId,a=e[0],o=this._compositeLayers(e,null),r=new Set(e.map(d=>d.id)),n=t.filter(d=>!r.has(d.id)||d.id===a.id).map(d=>d.id===a.id?{...d,canvas:o,opacity:1,blendMode:"normal"}:d),h=n.some(d=>d.id===i)?i:a.id;this._state={...this._state,layers:n,activeLayerId:h};const l=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:s,afterLayers:l,previousActiveLayerId:i,afterActiveLayerId:h}),this._markDirty()},flattenImage:()=>{if(this._state.layers.length<=1)return;this.canvas?.clearSelection();const t=this._snapshotAllLayers(),e=this._state.activeLayerId,s=this._state.layers.filter(n=>n.visible),i=s.length>0?s[0]:this._state.layers[0],a=this._compositeLayers(s),o={id:i.id,name:i.name,visible:!0,opacity:1,blendMode:"normal",canvas:a};this._state={...this._state,layers:[o],activeLayerId:o.id};const r=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:t,afterLayers:r,previousActiveLayerId:e,afterActiveLayerId:o.id}),this._markDirty()},toggleLayersPanel:()=>{this._state={...this._state,layersPanelOpen:!this._state.layersPanelOpen},this._markDirty("setting")},setCropAspectRatio:t=>{this._state={...this._state,cropAspectRatio:t},this._markDirty("setting")},setFontFamily:t=>{this._state={...this._state,fontFamily:t},this._markDirty("setting")},setFontSize:t=>{const e=Number.isFinite(t)?t:8;this._state={...this._state,fontSize:Math.max(8,Math.min(200,e))},this._markDirty("setting")},setFontBold:t=>{this._state={...this._state,fontBold:t},this._markDirty("setting")},setFontItalic:t=>{this._state={...this._state,fontItalic:t},this._markDirty("setting")},setBrush:t=>{this._updateBrush(t)},setBrushTip:t=>{this._updateBrush({tip:{...this._state.brush.tip,...t}})},setBrushInk:t=>{this._updateBrush({ink:{...this._state.brush.ink,...t}})},selectPreset:t=>{const e=Ri(t);if(!e)return;const s=e.descriptor;this._state={...this._state,brush:{...s,tip:{...s.tip},ink:{...s.ink}},activePreset:t,isPresetModified:!1},this._markDirty("setting")},setEyedropperSampleAll:t=>{this._state={...this._state,eyedropperSampleAll:t},this._markDirty("setting")},canUndo:this._canUndo,canRedo:this._canRedo,currentProject:this._currentProject,projectList:this._projectList,saving:this._saving,zoom:this._viewportZoom,panX:this._viewportPanX,panY:this._viewportPanY,viewportWidth:this._viewportWidth,viewportHeight:this._viewportHeight,isMobile:this._isMobile,switchProject:t=>{if(t===this._currentProject?.id)return;(async()=>{this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();const s=this._projectList.find(i=>i.id===t);s&&await this._enterProject(s,()=>this._loadProject(t))})().catch(s=>console.error("Switch project failed:",s))},createProject:(t,e,s)=>{(async()=>{this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();const a=await this._backend.projects.create({name:t,thumbnailRef:null});await this._enterProject(a,async()=>{this._projectList=await this._backend.projects.list(),await this._resetToFreshProject(e,s),this.canvas?.resetView()}),this._markDirty()})().catch(a=>console.error("Create project failed:",a))},deleteProject:t=>{(async()=>{if(this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait(),await this._projectService.deleteProject(t),this._projectList=await this._backend.projects.list(),t===this._currentProject?.id)if(this._projectList.length>0){const s=this._projectList[0];await this._enterProject(s,()=>this._loadProject(s.id))}else{const s=await this._backend.projects.create({name:"Untitled",thumbnailRef:null});this._projectList=[s],await this._enterProject(s,async()=>{await this._resetToFreshProject(),this.canvas?.resetView()}),this._markDirty()}})().catch(s=>console.error("Delete project failed:",s))},renameProject:(t,e)=>{(async()=>{const i=await this._backend.projects.update(t,{name:e});this._currentProject?.id===t&&(this._currentProject=i),this._projectList=await this._backend.projects.list()})().catch(i=>console.error("Rename project failed:",i))},transformActive:this.canvas?.isTransformActive()??!1,getTransformValues:()=>this.canvas?.getTransformValues()??null,setTransformValue:(t,e)=>this.canvas?.setTransformValue(t,e),setChildMode:t=>{this._state={...this._state,childMode:t},t&&!oa.has(this._state.activeTool)&&(this._state={...this._state,activeTool:"pencil"}),this._markDirty("setting")}}}willUpdate(){this._provider.setValue(this._buildContextValue()),this.toggleAttribute("mobile",this._isMobile)}_onHistoryChange(t){this._canUndo=t.detail.canUndo,this._canRedo=t.detail.canRedo,this._markDirty(),this._reportModified()}whenReady(){return this._ready}openImage(t,e={}){return this._replaceDocumentInTurn(async()=>{const s=await createImageBitmap(t);try{const i=document.createElement("canvas");i.width=s.width,i.height=s.height;const a=!!i.getContext("2d");if(i.width=i.height=0,!a)throw new RangeError(`This browser cannot open a ${s.width}×${s.height} image`);await this._replaceDocument(s.width,s.height,null,e.name??"Untitled",o=>o.canvas.getContext("2d").drawImage(s,0,0))}finally{s.close()}})}newDocument(t,e,s={}){return this._replaceDocumentInTurn(()=>this._replaceDocument(t,e,s.background===void 0?"#ffffff":s.background,s.name??"Untitled"))}async exportImage(t={}){await this._ready,await this._replacementsSettled();const e=t.type??"image/png",s=t.background!==void 0?t.background:e==="image/jpeg"?"#ffffff":null;this.canvas.clearSelection();const i=this.canvas.renderFlattened(s),a=this._markDocument();return this._exportedDocument=a,new Promise((o,r)=>{i.toBlob(n=>{if(!n){r(new Error(`Could not encode the image as ${e}`));return}this._exportMarks.set(n,a),o(n)},e,t.quality)})}get modified(){if(!this.canvas)return!1;if(this.canvas.isTransformActive()||this.canvas.hasPendingText())return!0;const t=this._historyTop();return t!==this._savedDocument.top?!0:t===null&&this.canvas.getHistoryTrimmedCount()!==this._savedDocument.trimmed}markSaved(t){const e=t?this._exportMarks.get(t):void 0;e&&e.generation!==this._documentGeneration||(this._savedDocument=e??this._exportedDocument??this._markDocument(),this._exportedDocument=null,this._reportModified())}async _showWholeDocument(){await this.updateComplete;const t=this.canvas;t&&(await t.updateComplete,t.resetView())}_historyTop(){const t=this.canvas?.getHistoryIndex()??-1;return t>=0?this.canvas.getHistory()[t]??null:null}_markDocument(){return{top:this._historyTop(),trimmed:this.canvas?.getHistoryTrimmedCount()??0,generation:this._documentGeneration}}async _replacementsSettled(){let t;do t=this._documentReplacement,await t;while(t!==this._documentReplacement)}_markSaved(){this._documentGeneration++,this._savedDocument=this._markDocument(),this._exportedDocument=null,this._reportModified()}_replaceDocumentInTurn(t){const e=this._documentReplacement.then(async()=>{await this._ready,await t(),await this._showWholeDocument(),this._markDirty(),this._markSaved()});return this._documentReplacement=e.catch(()=>{}),e}_reportModified(){const t=this.modified;t!==this._lastReportedModified&&(this._lastReportedModified=t,this.dispatchEvent(new CustomEvent("modified-change",{detail:{modified:t},bubbles:!0,composed:!0})))}_requestSave(){if(!this.embedded){this.canvas?.saveCanvas();return}this.canvas?.clearSelection(),this.dispatchEvent(new CustomEvent("save-request",{bubbles:!0,composed:!0}))}async _replaceDocument(t,e,s,i,a){if(t=Math.round(t),e=Math.round(e),!(t>0&&e>0&&t<=16384&&e<=16384))throw new RangeError(`Document size ${t}×${e} is outside 1–16384 pixels`);this.canvas?.cancelCrop(),this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();const r=this._currentProject,n=await this._backend.projects.create({name:i,thumbnailRef:null});if(await this._enterProject(n,async()=>{await this._resetToFreshProject(t,e,s),a&&(a(this._state.layers[0]),this.canvas?.composite())}),this.embedded&&r)try{await this._projectService.deleteProject(r.id)}catch(c){console.warn("Could not discard the previous document:",c)}this._projectList=await this._backend.projects.list()}_onViewportChange(){if(this.canvas){const t=this.canvas.getViewport();this._viewportZoom=t.zoom,this._viewportPanX=t.panX,this._viewportPanY=t.panY,this._viewportWidth=this.canvas.clientWidth,this._viewportHeight=this.canvas.clientHeight}this._markDirty("viewport")}_onTransformChange(){this.requestUpdate(),this._reportModified()}_onNavigatorPan(t){if(!this.canvas)return;const{panX:e,panY:s}=t.detail,i=this.canvas.getViewport();this.canvas.setViewport(i.zoom,e,s)}_onNavigatorZoom(t){if(!this.canvas)return;const e=t.detail.zoom,s=this.canvas.getViewport(),i=this.canvas.clientWidth/2,a=this.canvas.clientHeight/2,o=(i-s.panX)/s.zoom,r=(a-s.panY)/s.zoom,n=i-o*e,c=a-r*e;this.canvas.setViewport(e,n,c)}_onLayerUndo(t){const e=t.detail;switch(e.action){case"remove-layer":{const s=this._state.layers.findIndex(o=>o.id===e.layerId),i=this._state.layers.filter(o=>o.id!==e.layerId);if(i.length===0)return;const a=this._state.activeLayerId===e.layerId?i[Math.min(Math.max(0,s-1),i.length-1)].id:this._state.activeLayerId;this._state={...this._state,layers:i,activeLayerId:a};break}case"restore-layer":{const s=e.snapshot,i=this._state.documentWidth,a=this._state.documentHeight,o=document.createElement("canvas");o.width=i,o.height=a,o.getContext("2d").putImageData(s.imageData,0,0);const r={id:s.id,name:s.name,visible:s.visible,opacity:s.opacity,blendMode:s.blendMode??"normal",canvas:o},n=[...this._state.layers],c=e.index===-1?n.length:e.index;n.splice(c,0,r);const h=n.some(l=>l.id===this._state.activeLayerId);this._state={...this._state,layers:n,activeLayerId:h?this._state.activeLayerId:r.id};break}case"reorder":{const s=[...this._state.layers];if(e.fromIndex<0||e.fromIndex>=s.length||e.toIndex<0||e.toIndex>=s.length)break;const[i]=s.splice(e.fromIndex,1);s.splice(e.toIndex,0,i),this._state={...this._state,layers:s};break}case"refresh":{this._state={...this._state,layers:[...this._state.layers]};break}case"crop-restore":{const s=e.layers,i=e.width,a=e.height,o=this._state.layers.map(r=>{const n=s.find(h=>h.id===r.id);if(!n)return r;const c=document.createElement("canvas");return c.width=n.imageData.width,c.height=n.imageData.height,c.getContext("2d").putImageData(n.imageData,0,0),{...r,canvas:c,visible:n.visible,opacity:n.opacity,blendMode:n.blendMode??"normal",name:n.name}});this._applyDocumentDimensions(i,a),this._state={...this._state,layers:o};break}case"stack-replace":{const s=e.layers,i=e.activeLayerId,a=s.map(o=>{const r=document.createElement("canvas");return r.width=o.imageData.width,r.height=o.imageData.height,r.getContext("2d").putImageData(o.imageData,0,0),{id:o.id,name:o.name,visible:o.visible,opacity:o.opacity,blendMode:o.blendMode??"normal",canvas:r}});this._state={...this._state,layers:a,activeLayerId:i};break}}this._markDirty()}_updateMobileLayout(t){const e=go(t,this._isMobile);e!==this._isMobile&&(this._isMobile=e,e?(this._desktopLayersPanelOpen=this._state.layersPanelOpen,this._state={...this._state,layersPanelOpen:!1}):this._desktopLayersPanelOpen!==null&&(this._state={...this._state,layersPanelOpen:this._desktopLayersPanelOpen},this._desktopLayersPanelOpen=null),!e&&this._state.childMode&&(this._state={...this._state,childMode:!1}))}connectedCallback(){super.connectedCallback(),this._initStorage(),this._mobileObserver=new ResizeObserver(t=>{for(const e of t)this._updateMobileLayout(e.contentRect.width)}),this._mobileObserver.observe(this),this.addEventListener("keydown",this._onKeyDown),window.addEventListener("beforeunload",this._onBeforeUnload),document.addEventListener("visibilitychange",this._onVisibilityChange)}_initStorage(){this._initPromise||(this._initPromise=this._doInitStorage())}async _doInitStorage(){try{const t=!!this.storageBackend,e=this.storageBackend??(this.embedded?new Yi:new Ki);await e.init(),this._backend=e,this._ownsBackend=!t,this._autosave=!(this.embedded&&!t),this._projectService=new zi(e),this._storageProvider=new ye(this,{context:Os,initialValue:this._backend}),this._serviceProvider=new ye(this,{context:js,initialValue:this._projectService}),this._storageState="ready",await this._bootstrapProjects(),this._markSaved(),this._resolveReady()}catch(t){this._rejectReady(t),console.error("Storage initialization failed:",t),this._storageState="error",this._storageError="Could not open local storage. Try reloading or checking browser storage settings."}}async _bootstrapProjects(){if(this._projectList=await this._backend.projects.list(),this._projectList.length>0){const t=this._projectList[0];await this._enterProject(t,()=>this._loadProject(t.id))}else{const t=await this._backend.projects.create({name:"Untitled",thumbnailRef:null});this._currentProject=t,this._projectList=[t],this._markDirty(),await this.updateComplete,await this.canvas?.updateComplete,this.canvas?.resetView()}}disconnectedCallback(){if(super.disconnectedCallback(),this._mobileObserver?.disconnect(),this._mobileObserver=null,this.removeEventListener("keydown",this._onKeyDown),window.removeEventListener("beforeunload",this._onBeforeUnload),document.removeEventListener("visibilitychange",this._onVisibilityChange),this._dirty||this._savePromise){const t=this._ownsBackend?this._backend:void 0;(this._dirty?this._flushPendingSaveAndWait():this._savePromise).finally(()=>t?.dispose())}else this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),this._ownsBackend&&this._backend?.dispose()}render(){return this._storageState==="loading"?g`<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#888;">Loading...</div>`:this._storageState==="error"?g`<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:100%;color:#ff6b6b;gap:8px;">
        <p>Failed to initialize storage</p>
        <p style="font-size:0.85em;color:#999;">${this._storageError}</p>
      </div>`:g`
      ${this._isMobile?"":g`<tool-settings></tool-settings>`}
      <div class="main-area">
        <app-toolbar></app-toolbar>
        <drawing-canvas
          @history-change=${this._onHistoryChange}
          @layer-undo=${this._onLayerUndo}
          @crop-commit=${this._onCropCommit}
          @transform-change=${this._onTransformChange}
          @pending-text-change=${this._reportModified}
          @viewport-change=${this._onViewportChange}
        ></drawing-canvas>
        ${this._isMobile?"":g`
          <div class="right-sidebar ${this._state.layersPanelOpen?"":"collapsed"}">
            <navigator-panel
              @navigator-pan=${this._onNavigatorPan}
              @navigator-zoom=${this._onNavigatorZoom}
            ></navigator-panel>
            <layers-panel @commit-opacity=${this._onCommitOpacity}></layers-panel>
          </div>
        `}
      </div>
      ${this._isMobile&&!this._state.childMode?g`<layers-panel @commit-opacity=${this._onCommitOpacity}></layers-panel>`:""}
    `}};D.styles=bt`
    :host {
      display: flex;
      flex-direction: column;
      /* The document's border-box rule does not cross the shadow boundary, so
         set it here: safe-area padding must fit inside the 100% height. */
      box-sizing: border-box;
      width: 100%;
      height: 100%;
      /* Keep the UI clear of the status bar and home indicator when installed
         to the home screen (viewport-fit=cover). */
      padding-top: env(safe-area-inset-top);
      padding-left: env(safe-area-inset-left);
      padding-right: env(safe-area-inset-right);
      background: #1e1e1e;
      font-family: system-ui, -apple-system, sans-serif;
      position: relative;
    }

    /* The mobile toolbar and layers panel pad their own bottom inset. */
    :host(:not([mobile])) {
      padding-bottom: env(safe-area-inset-bottom);
    }

    .main-area {
      display: flex;
      flex: 1;
      min-height: 0;
    }

    drawing-canvas {
      flex: 1;
    }

    .right-sidebar {
      display: flex;
      flex-direction: column;
      overflow: hidden;
      height: 100%;
      width: 200px;
      border-left: 1px solid #444;
      background: #2c2c2c;
      transition: width 0.2s ease;
    }

    .right-sidebar.collapsed {
      width: 32px;
    }

    .right-sidebar layers-panel {
      flex: 1;
      min-height: 0;
    }

    /* ── Mobile layout ─────────────────────────── */
    :host([mobile]) {
      flex-direction: column;
    }


    :host([mobile]) .main-area {
      flex-direction: column;
    }


    :host([mobile]) .main-area app-toolbar {
      order: 1;
    }
  `;D.THUMBNAIL_SIZE=256;D.NON_TEXT_INPUT_TYPES=new Set(["button","checkbox","color","file","hidden","image","radio","range","reset","submit"]);z([M()],D.prototype,"_state",2);z([M()],D.prototype,"_canUndo",2);z([M()],D.prototype,"_canRedo",2);z([M()],D.prototype,"_saving",2);z([M()],D.prototype,"_viewportZoom",2);z([M()],D.prototype,"_viewportPanX",2);z([M()],D.prototype,"_viewportPanY",2);z([M()],D.prototype,"_viewportWidth",2);z([M()],D.prototype,"_viewportHeight",2);z([M()],D.prototype,"_currentProject",2);z([M()],D.prototype,"_projectList",2);z([M()],D.prototype,"_isMobile",2);z([Be({attribute:!1})],D.prototype,"storageBackend",2);z([Be({type:Boolean,reflect:!0})],D.prototype,"embedded",2);z([M()],D.prototype,"_storageState",2);z([M()],D.prototype,"_storageError",2);z([M()],D.prototype,"_backend",2);z([M()],D.prototype,"_projectService",2);z([fe("drawing-canvas")],D.prototype,"canvas",2);D=z([yt("drawing-app")],D);export{Ft as AppToolbar,D as DrawingApp,R as DrawingCanvas,Ki as IndexedDBBackend,Yi as MemoryBackend,j as ToolSettings,Et as drawingContext};
